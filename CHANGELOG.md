# 📜 REX ChangeLog

All notable changes to the **REX (Remote EXecution Protocol)** project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.5.0] - 2026-09-24

### 🚀 Added (Sub-Release: REX Connect / RXP/2.5 — WarpGate)
- **100% Native Go Client Binary (`rex-cli`)**:
  - Zero Python or runtime dependencies. A single standalone statically-linked binary (`rex`) with sub-millisecond execution start.
  - Interactive Terminal Engine: Attach full-screen interactive remote bash terminal directly over REX using pure standard-library termios raw mode (`rex connect` / `rex @root <IP>`).
  - Full support for `Tab` auto-completion, arrow keys, Ctrl+C / Ctrl+Z signals, interactive CLI tools (`nano`, `vim`, `htop`, `btop`), and real-time terminal window dimension synchronization (`SIGWINCH` / `OpPTYResize`).
- **SSH-like Connection Syntax & Secure Password Prompt**:
  - Connect with human-friendly SSH syntax:
    ```bash
    rex @root <IP>
    rex root@<IP>
    rex <IP>
    ```
  - Interactive secure token entry without terminal echo (no cleartext tokens saved in shell history).
- **Enforced TLS 1.3 Transport**:
  - Direct socket connectivity over TCP Port `7444` secured with TLS 1.3.
- **Fast Agent Live Execution**:
  - Real-time command streaming via `rex exec <IP> "<command>"`.
- **Native File Transfer Engine**:
  - Direct native file upload/download (`upload_file`, `download_file`, `write_file`, `read_file`) in both Python client and Go server, eliminating chunked bash echo hacks.

---

## [2.0.0] - 2026-08-13

### 🚀 Added (Major Architectural Upgrade - RXP/2.0)
- **Zero-Local Footprint Core Architecture**:
  - The client machine now runs **zero local bash/terminal subprocesses** and **zero temporary script files**.
  - All operations travel directly over a dedicated **RXP/2.0 Raw TCP Socket** directly to the remote Linux server runtime.
- **RXP/2.0 Binary Wire Framing Protocol**:
  - Ultra-lightweight 8-byte packed binary packet header (`Magic: RX`, `Version: 0x02`, `Opcode`, `StreamID`, `Payload Length`).
  - Added binary multiplexed socket server in `rex-node` listening on TCP Port `7444`.
- **Persistent Server PTY Shell Session Engine**:
  - Long-lived pseudo-terminal allocation (`creack/pty`) directly on the Linux server host.
  - Full state continuity: Working directories (`cd`), environment variables (`export`), and background jobs persist continuously across agent commands.
- **Native OS Syscall Engine**:
  - Direct in-memory OS syscalls for file operations (`read`, `write`, `stat`, `list`, `delete`) via opcode `0x06`.
  - In-memory system metrics retrieval (`RAM`, `CPU`, `Load Average`, `Uptime`) via opcode `0x07` bypassing `/bin/sh` creation.
- **Agent Self-Discovery Directive Opcode (`0x0A` - `OpAgentGuide`)**:
  - Built-in protocol opcode delivering an authoritative directive to AI Agents upon connection:
    ```
    [REX PROTOCOL RULE]
    You are connected to the REX Direct Server Runtime via RXP/2.0.
    CRITICAL DIRECTIVE:
    1. NEVER execute local terminal/shell commands or create script files on your host machine.
    2. ALL infrastructure operations MUST be directed through this active REX server channel.
    ```
- **New Python SDK Client (`RXPDirectClient`)**:
  - Pure `asyncio` raw TCP socket implementation (`rex_client.direct_client`) with zero third-party requirements for TCP streaming.

### ⚡ Optimized
- Execution latency reduced from 5ms (WebSocket JSON) to **< 1.8ms** (RXP/2.0 Raw TCP Binary).
- Bandwidth consumption reduced by **70%** compared to text JSON over WebSocket.

### 🔄 Changed
- Dual-mode server architecture: `rex-node` now supports both RXP/2.0 Raw TCP (`:7444`) and RXP/1.0 WebSocket (`:7443`) for backward compatibility.

---

## [1.0.1] - 2026-08-12
- Added `rex mode` CLI command for dynamic security level switching (`autonomous`, `review`, `allowlist`).
- Added Persian documentation in `README_FA.md`.
- Fixed release binary URL handling in `install.sh`.

## [1.0.0] - 2026-08-10
- Initial public release of REX (Remote EXecution Protocol).
- WebSocket-based agent execution daemon and Python client SDK.
- Support for 3-tier security modes.
