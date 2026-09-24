"""
REXClient — async Python client for the Remote EXecution Protocol (RXP).
"""

import asyncio
import json
import logging
import ssl
import uuid
from typing import AsyncIterator, Optional, Union

try:
    import websockets
    from websockets.exceptions import ConnectionClosed
except ImportError:
    websockets = None
    ConnectionClosed = Exception

from rex_client.exceptions import (
    REXAuthError,
    REXCommandDenied,
    REXConnectionError,
    REXProtocolError,
    REXTimeout,
)
from rex_client.models import ExecResult, HandshakeInfo, SysinfoResult

logger = logging.getLogger("rex_client")


class REXClient:
    """
    Async client for REX (Remote EXecution Protocol).

    Usage (context manager):
        async with REXClient("1.2.3.4", token="secret") as node:
            result = await node.exec("systemctl status xray")
            print(result.stdout)
    """

    def __init__(
        self,
        host: str,
        token: str,
        port: int = 7443,
        tls: bool = False,
        tls_verify: bool = True,
        timeout: float = 30.0,
        client_id: str = "rex-python-client",
    ):
        self.host = host
        self.port = port
        self.token = token
        self.tls = tls
        self.tls_verify = tls_verify
        self.default_timeout = timeout
        self.client_id = client_id

        self._ws = None
        self._server_info: Optional[HandshakeInfo] = None
        self._pending: dict[str, asyncio.Future] = {}
        self._stream_queues: dict[str, asyncio.Queue] = {}
        self._reader_task: Optional[asyncio.Task] = None
        self._lock = asyncio.Lock()

    @property
    def url(self) -> str:
        scheme = "wss" if self.tls else "ws"
        return f"{scheme}://{self.host}:{self.port}/rex"

    @property
    def server_info(self) -> Optional[HandshakeInfo]:
        """Server metadata received during RXP handshake."""
        return self._server_info

    @property
    def is_connected(self) -> bool:
        return self._ws is not None and not getattr(self._ws, "closed", False)

    async def connect(self) -> HandshakeInfo:
        """Open WebSocket connection and perform RXP handshake."""
        if self.is_connected:
            return self._server_info

        ssl_ctx = None
        if self.tls:
            ssl_ctx = ssl.create_default_context()
            if not self.tls_verify:
                ssl_ctx.check_hostname = False
                ssl_ctx.verify_mode = ssl.CERT_NONE

        logger.info("Connecting to %s", self.url)
        try:
            self._ws = await websockets.connect(self.url, ssl=ssl_ctx)
        except Exception as exc:
            raise REXConnectionError(f"Cannot connect to {self.url}: {exc}") from exc

        # Perform RXP Handshake
        await self._send({
            "type": "handshake",
            "version": "1.0",
            "client_id": self.client_id,
            "token": self.token,
        })

        try:
            raw = await asyncio.wait_for(self._ws.recv(), timeout=10.0)
        except asyncio.TimeoutError:
            await self._ws.close()
            raise REXTimeout("Handshake timed out")

        msg = json.loads(raw)
        if msg.get("type") != "handshake_ack":
            raise REXProtocolError(f"Expected handshake_ack, got: {msg.get('type')}")
        if msg.get("status") != "ok":
            raise REXAuthError(f"Authentication failed: {msg.get('error', 'unknown')}")

        self._server_info = HandshakeInfo(
            node_id=msg.get("node_id", "unknown"),
            os=msg.get("os", "unknown"),
            capabilities=msg.get("capabilities", []),
            protocol=msg.get("protocol", "RXP/2.5"),
            agent_guide=msg.get("agent_guide"),
        )
        logger.info("Connected to %s (%s, protocol=%s)", self._server_info.node_id, self._server_info.os, self._server_info.protocol)

        self._reader_task = asyncio.create_task(self._reader_loop())
        return self._server_info

    async def disconnect(self):
        """Close connection gracefully."""
        if self._reader_task:
            self._reader_task.cancel()
            try:
                await self._reader_task
            except asyncio.CancelledError:
                pass
            self._reader_task = None

        if self._ws and not getattr(self._ws, "closed", False):
            await self._ws.close()
        self._ws = None
        logger.info("Disconnected from %s", self.host)

    async def __aenter__(self) -> "REXClient":
        await self.connect()
        return self

    async def __aexit__(self, *_):
        await self.disconnect()

    async def exec(
        self,
        command: str,
        timeout: Optional[float] = None,
    ) -> ExecResult:
        """Execute a command on remote server via REX."""
        self._ensure_connected()
        msg_id = _new_id("cmd")
        timeout = timeout or self.default_timeout

        future: asyncio.Future = asyncio.get_event_loop().create_future()
        self._pending[msg_id] = future

        await self._send({
            "type": "exec",
            "id": msg_id,
            "command": command,
            "timeout": int(timeout),
        })

        try:
            response = await asyncio.wait_for(future, timeout=timeout + 5)
        except asyncio.TimeoutError:
            self._pending.pop(msg_id, None)
            raise REXTimeout(f"exec timed out after {timeout}s: {command!r}")

        result = ExecResult(
            exit_code=response.get("exit_code", -1),
            stdout=response.get("stdout", ""),
            stderr=response.get("stderr", ""),
            duration_ms=response.get("duration_ms", 0),
            error=response.get("error") or None,
        )

        if result.error and "not allowed" in result.error:
            raise REXCommandDenied(f"Command denied by server policy: {command!r}")

        return result

    async def sysinfo(self, timeout: float = 10.0) -> SysinfoResult:
        """Retrieve system metrics from remote node."""
        self._ensure_connected()
        msg_id = _new_id("info")

        future: asyncio.Future = asyncio.get_event_loop().create_future()
        self._pending[msg_id] = future

        await self._send({"type": "sysinfo", "id": msg_id})

        try:
            response = await asyncio.wait_for(future, timeout=timeout)
        except asyncio.TimeoutError:
            self._pending.pop(msg_id, None)
            raise REXTimeout("sysinfo timed out")

        return SysinfoResult(
            os=response.get("os", ""),
            arch=response.get("arch", ""),
            cpus=response.get("cpus", 0),
            mem_total_kb=response.get("mem_total_kb", 0),
            mem_available_kb=response.get("mem_available_kb", 0),
            mem_free_kb=response.get("mem_free_kb", 0),
            uptime_seconds=response.get("uptime_seconds", 0.0),
            load_1m=response.get("load_1m", 0.0),
            load_5m=response.get("load_5m", 0.0),
            load_15m=response.get("load_15m", 0.0),
        )

    async def stream(
        self,
        path: str,
        tail: int = 50,
    ) -> AsyncIterator[str]:
        """Stream a log file from remote node line by line."""
        self._ensure_connected()
        msg_id = _new_id("stream")
        queue: asyncio.Queue[Optional[str]] = asyncio.Queue()
        self._stream_queues[msg_id] = queue

        await self._send({
            "type": "stream",
            "id": msg_id,
            "target": path,
            "lines": tail,
        })

        try:
            while True:
                line = await queue.get()
                if line is None:
                    break
                yield line
        finally:
            self._stream_queues.pop(msg_id, None)
            try:
                await self._send({"type": "stream_stop", "id": msg_id})
            except Exception:
                pass

    async def write_file(
        self,
        remote_path: str,
        content: Union[str, bytes],
        mode: int = 0o644,
        append: bool = False,
        timeout: float = 30.0,
    ) -> int:
        """Write string or bytes directly to a remote file via REX native file protocol."""
        self._ensure_connected()
        import base64

        if isinstance(content, str):
            content_bytes = content.encode("utf-8")
        else:
            content_bytes = content

        b64_data = base64.b64encode(content_bytes).decode("ascii")
        msg_id = _new_id("fwr")
        future: asyncio.Future = asyncio.get_event_loop().create_future()
        self._pending[msg_id] = future

        await self._send({
            "type": "file_write",
            "id": msg_id,
            "path": remote_path,
            "data": b64_data,
            "mode": mode,
            "append": append,
        })

        try:
            response = await asyncio.wait_for(future, timeout=timeout)
        except asyncio.TimeoutError:
            self._pending.pop(msg_id, None)
            raise REXTimeout(f"write_file timed out for {remote_path}")

        # If server is older REX 1.0 (returns unknown type: file_write), fallback gracefully!
        if response.get("type") == "error" and "unknown type" in response.get("error", ""):
            logger.info("Server does not support native file_write; using chunked pipe fallback")
            return await self._fallback_write_file(remote_path, content_bytes, mode=mode, append=append)

        if response.get("status") != "ok":
            raise REXProtocolError(f"file_write failed: {response.get('error')}")

        return response.get("bytes_written", len(content_bytes))

    async def _fallback_write_file(self, remote_path: str, data: bytes, mode: int = 0o644, append: bool = False) -> int:
        """Fallback chunked write using base64 for legacy REX nodes without native file_write."""
        import base64
        chunk_size = 4096
        # If not appending, truncate file first
        if not append:
            await self.exec(f": > '{remote_path}'")

        for i in range(0, len(data), chunk_size):
            chunk = data[i:i + chunk_size]
            b64_chunk = base64.b64encode(chunk).decode("ascii")
            cmd = f"printf '%s' '{b64_chunk}' | base64 -d >> '{remote_path}'"
            res = await self.exec(cmd, timeout=10)
            if res.exit_code != 0:
                raise REXProtocolError(f"Fallback write failed: {res.stderr}")

        await self.exec(f"chmod {oct(mode)[2:]} '{remote_path}'")
        return len(data)

    async def read_file(self, remote_path: str, timeout: float = 30.0) -> bytes:
        """Read a file directly from remote server."""
        self._ensure_connected()
        import base64
        msg_id = _new_id("frd")
        future: asyncio.Future = asyncio.get_event_loop().create_future()
        self._pending[msg_id] = future

        await self._send({
            "type": "file_read",
            "id": msg_id,
            "path": remote_path,
        })

        try:
            response = await asyncio.wait_for(future, timeout=timeout)
        except asyncio.TimeoutError:
            self._pending.pop(msg_id, None)
            raise REXTimeout(f"read_file timed out for {remote_path}")

        if response.get("type") == "error" and "unknown type" in response.get("error", ""):
            res = await self.exec(f"base64 '{remote_path}'")
            if res.exit_code != 0:
                raise REXProtocolError(f"read_file fallback failed: {res.stderr}")
            return base64.b64decode(res.stdout.strip())

        if response.get("status") != "ok":
            raise REXProtocolError(f"read_file failed: {response.get('error')}")

        return base64.b64decode(response.get("data", ""))

    async def upload_file(self, local_path: str, remote_path: str, mode: int = 0o644, timeout: float = 60.0) -> int:
        """Upload a local file to remote server."""
        with open(local_path, "rb") as f:
            data = f.read()
        return await self.write_file(remote_path, data, mode=mode, timeout=timeout)

    async def download_file(self, remote_path: str, local_path: str, timeout: float = 60.0) -> int:
        """Download a remote file to local machine."""
        data = await self.read_file(remote_path, timeout=timeout)
        with open(local_path, "wb") as f:
            f.write(data)
        return len(data)

    async def ping(self) -> float:
        """Ping remote node and return RTT in milliseconds."""
        self._ensure_connected()
        import time
        msg_id = _new_id("ping")
        future: asyncio.Future = asyncio.get_event_loop().create_future()
        self._pending[msg_id] = future

        t0 = time.monotonic()
        await self._send({"type": "ping", "id": msg_id})

        try:
            await asyncio.wait_for(future, timeout=5.0)
        except asyncio.TimeoutError:
            self._pending.pop(msg_id, None)
            raise REXTimeout("ping timed out")

        return round((time.monotonic() - t0) * 1000, 2)

    def _ensure_connected(self):
        if not self.is_connected:
            raise REXConnectionError("Not connected. Call connect() first.")

    async def _send(self, data: dict):
        async with self._lock:
            await self._ws.send(json.dumps(data))

    async def _reader_loop(self):
        try:
            async for raw in self._ws:
                try:
                    msg = json.loads(raw)
                except json.JSONDecodeError:
                    logger.warning("Received non-JSON message")
                    continue

                msg_type = msg.get("type", "")
                msg_id = msg.get("id", "")

                if msg_type in ("exec_result", "sysinfo_result", "file_write_result", "file_read_result", "pong", "error"):
                    self._resolve_future(msg_id, msg)

                elif msg_type == "stream_line":
                    q = self._stream_queues.get(msg_id)
                    if q:
                        await q.put(msg.get("line", ""))

                elif msg_type in ("stream_end", "stream_error"):
                    q = self._stream_queues.pop(msg_id, None)
                    if q:
                        if msg_type == "stream_error":
                            logger.error("Stream error for %s: %s", msg_id, msg.get("error"))
                        await q.put(None)

        except ConnectionClosed:
            logger.info("Connection closed by server")
        except asyncio.CancelledError:
            pass
        except Exception as exc:
            logger.error("Reader loop error: %s", exc)
        finally:
            for fut in self._pending.values():
                if not fut.done():
                    fut.set_exception(REXConnectionError("Connection lost"))
            self._pending.clear()
            for q in self._stream_queues.values():
                await q.put(None)
            self._stream_queues.clear()

    def _resolve_future(self, msg_id: str, msg: dict):
        future = self._pending.pop(msg_id, None)
        if future and not future.done():
            future.set_result(msg)


def _new_id(prefix: str) -> str:
    return f"{prefix}_{uuid.uuid4().hex[:8]}"
