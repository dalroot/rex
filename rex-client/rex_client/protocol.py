"""
RXP/2.0 Binary Frame Protocol Encoder and Decoder.
"""

import struct
from typing import NamedTuple, Optional

MAGIC_BYTES = b"RX"
VERSION_2 = 0x02

# RXP Opcodes
OP_AUTH_HANDSHAKE = 0x01
OP_PTY_SPAWN = 0x02
OP_PTY_DATA = 0x03
OP_PTY_RESIZE = 0x04
OP_PTY_CLOSE = 0x05
OP_NATIVE_FILE_OP = 0x06
OP_NATIVE_SYSINFO = 0x07
OP_PING = 0x08
OP_PONG = 0x09
OP_AGENT_GUIDE = 0x0A
OP_FAST_EXEC = 0x0B
OP_ERROR = 0xFF


class RXPFrame(NamedTuple):
    version: int
    opcode: int
    stream_id: int
    payload: bytes

    def encode(self) -> bytes:
        payload_len = len(self.payload)
        if payload_len > 65535:
            raise ValueError("Payload exceeds max 65535 bytes")

        header = struct.pack(
            "!2sBBHH",
            MAGIC_BYTES,
            self.version,
            self.opcode,
            self.stream_id,
            payload_len,
        )
        return header + self.payload


async def read_rxp_frame(reader) -> Optional[RXPFrame]:
    """Read an RXP/2.0 binary frame from an asyncio.StreamReader."""
    header = await reader.readexactly(8)
    magic, ver, opcode, stream_id, payload_len = struct.unpack("!2sBBHH", header)

    if magic != MAGIC_BYTES:
        raise ValueError(f"Invalid magic bytes: {magic!r}")

    payload = b""
    if payload_len > 0:
        payload = await reader.readexactly(payload_len)

    return RXPFrame(
        version=ver,
        opcode=opcode,
        stream_id=stream_id,
        payload=payload,
    )
