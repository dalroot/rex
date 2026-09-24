"""Data models for REX messages."""

from dataclasses import dataclass
from typing import List, Optional


@dataclass
class HandshakeInfo:
    """Server info received after a successful RXP handshake."""
    node_id: str
    os: str
    capabilities: List[str]
    protocol: str = "RXP/2.5"
    agent_guide: Optional[str] = None


@dataclass
class ExecResult:
    """Result of a remote command execution."""
    exit_code: int
    stdout: str
    stderr: str
    duration_ms: int = 0
    error: Optional[str] = None

    @property
    def ok(self) -> bool:
        """True if the command exited with code 0."""
        return self.exit_code == 0

    def __str__(self) -> str:
        parts = [f"exit_code={self.exit_code}"]
        if self.stdout:
            parts.append(f"stdout={self.stdout!r}")
        if self.stderr:
            parts.append(f"stderr={self.stderr!r}")
        if self.error:
            parts.append(f"error={self.error!r}")
        return f"ExecResult({', '.join(parts)})"


@dataclass
class SysinfoResult:
    """System information from the remote node."""
    os: str = ""
    arch: str = ""
    cpus: int = 0
    mem_total_kb: int = 0
    mem_available_kb: int = 0
    mem_free_kb: int = 0
    uptime_seconds: float = 0.0
    load_1m: float = 0.0
    load_5m: float = 0.0
    load_15m: float = 0.0

    @property
    def mem_total_gb(self) -> float:
        return round(self.mem_total_kb / 1024 / 1024, 2)

    @property
    def mem_used_gb(self) -> float:
        used_kb = self.mem_total_kb - self.mem_available_kb
        return round(used_kb / 1024 / 1024, 2)

    @property
    def uptime_hours(self) -> float:
        return round(self.uptime_seconds / 3600, 1)

    def __str__(self) -> str:
        return (
            f"SysinfoResult("
            f"os={self.os}, cpus={self.cpus}, "
            f"mem={self.mem_used_gb}/{self.mem_total_gb}GB, "
            f"load={self.load_1m:.2f}, uptime={self.uptime_hours}h)"
        )
