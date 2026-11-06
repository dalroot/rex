# REX — Remote EXecution Protocol (RXP/2.5)

[![Release](https://img.shields.io/github/v/release/dalroot/rex?style=flat-square&color=2563eb&label=Release)](https://github.com/dalroot/rex/releases)
[![License](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](LICENSE)
[![Go Report](https://img.shields.io/badge/Language-Go%20%7C%20Python-00add8?style=flat-square)](https://golang.org)
[![Transport](https://img.shields.io/badge/Transport-TLS%201.3-success?style=flat-square)](https://en.wikipedia.org/wiki/Transport_Layer_Security)

An open-source, lightweight communication layer designed for autonomous AI agents and automated workflows to interact with remote Linux servers, execute commands, stream live terminal sessions, and transfer data without relying on OpenSSH handshakes or heavy shell wrappers.

---

> 🇮🇷 **راهنمای فارسی:** مستندات کامل به زبان فارسی در فایل [README_FA.md](README_FA.md) در دسترس است.

---

## Overview

Traditional remote execution approaches present clear trade-offs when automated by AI agents or programmatic pipelines:
- **OpenSSH / SSHPass**: Designed for human interaction; requires repeated cryptographic handshakes, PTY allocations on the client, shell environment re-initialization, and string escaping across nested commands.
- **HTTP / REST Endpoints**: Stateless; lacks persistent working directory state (`cd`, `export`), streaming stderr/stdout in real-time, or native PTY control for interactive tools.

**REX** bridges this gap by providing:
1. **Binary Framing (RXP/2.5)**: Low-overhead 8-byte frame header over encrypted TLS 1.3 TCP sockets.
2. **Fast Execution (`rex exec`)**: Direct command execution in isolated processes with immediate return of exit codes, stdout, and stderr.
3. **Interactive & Stateful Terminal (`rex connect`)**: Persistent remote pseudo-terminal (PTY) supporting ANSI escapes, dynamic window resize, and interactive tools (`vim`, `htop`, `nano`).
4. **Daemon Lifecycle Management**: In-place updates, systemd service control, and access policy management directly via the unified CLI.

---

## Comparison: REX vs. OpenSSH vs. HTTP/REST

| Capability | OpenSSH (`ssh` / `sshpass`) | HTTP REST APIs | REX Protocol (`rex`) |
| :--- | :--- | :--- | :--- |
| **Primary Target** | Human terminal operators & interactive sessions | Web services & microservices | Autonomous AI agents & automated pipelines |
| **Transport Overhead** | Full SSH negotiation & key exchange per session | HTTP headers, TLS handshakes, JSON serialization | 8-byte binary framing over persistent TLS 1.3 socket |
| **Command Execution Latency** | ~50ms – 250ms (session setup dependent) | ~15ms – 60ms (stateless request) | ~1.5ms – 5ms (FastExec direct dispatch) |
| **Local Client Footprint** | Spawns local bash + sshpass processes & local PTY | HTTP client runtime & dependencies | Statically-linked Go binary (`<15MB`), zero dependencies |
| **Stateful Working Directory** | Lost between separate non-interactive calls | Requires server-side session store & token tracking | Native PTY session maintains state across consecutive calls |
| **Agent Self-Discovery** | None (agent must guess OS & tools) | OpenAPI / Swagger schema (if enabled) | Structured JSON guide on port 7443 & built-in help |
| **Security Policy Modes** | User/Group UNIX permissions | API tokens & RBAC | 3 Node modes: `autonomous`, `review`, and `allowlist` |

---

## Architecture

REX consists of two core components:

```
┌──────────────────────────────────────┐             TLS 1.3 Socket (:7444)            ┌──────────────────────────────────────────────┐
│           REX Client (rex)           │ ═════════════════════════════════════════════ │           REX Daemon (rex-node)              │
│                                      │          8-Byte RXP Binary Framing            │                                              │
│  • rex exec    (Fast Command Exec)   │                                               │  • RXP Protocol Demux                        │
│  • rex connect (Interactive PTY)     │                                               │  • Subprocess Execution Engine               │
│  • rex info    (System Metrics)      │                                               │  • Stateful PTY Terminal Runtime             │
│  • rex update  (Daemon Lifecycle)    │                                               │  • Policy Filter (Autonomous/Review/Allow)   │
└──────────────────────────────────────┘                                               └──────────────────────────────────────────────┘
```

1. **`rex-node` (Server Daemon)**:
   - Written in Go. Runs as a systemd service on remote Linux hosts.
   - Listens on TCP port `7444` (encrypted with auto-generated TLS 1.3 certificates).
   - Manages subprocess execution, persistent PTY allocation via Linux pseudo-terminals, and policy-based command filtering.
   - Provides a discovery endpoint on port `7443` for agent self-configuration.

2. **`rex-cli` (Client Tool)**:
   - Statically compiled single binary (`rex`) with zero external dependencies.
   - Formats execution output with clean, readable terminal framing by default, with `-r` / `--raw` support for scripts and pipelines.

3. **`rex-client` (Python SDK)**:
   - Async Python client (`asyncio`) for direct programmatic agent integration.

---

## Quick Start

### 1. Server Installation (`rex-node`)

Run the automated installer on the remote Linux host (Ubuntu, Debian, CentOS, RHEL):

```bash
curl -fsSL https://raw.githubusercontent.com/dalroot/rex/master/rex-node/install.sh | bash
```

*The installer generates a secure token and TLS 1.3 certificate, configures systemd, and starts the service.*

To specify a custom token during setup:
```bash
curl -fsSL https://raw.githubusercontent.com/dalroot/rex/master/rex-node/install.sh | bash -s -- --token YOUR_TOKEN
```

### 2. Client Installation (`rex-cli`)

Install the standalone `rex` binary onto the client machine or agent runtime (Linux x86_64, ARM64):

```bash
curl -fsSL https://raw.githubusercontent.com/dalroot/rex/master/install-cli.sh | bash
```

---

## CLI Usage

### Fast Command Execution (`rex exec`)

Execute a remote command and stream output back:

```bash
rex exec <SERVER_IP>:7444 -t <TOKEN> "uptime"
```

Formatted output:
```text
┌── ⚡ REX [<SERVER_IP>:7444]
│ ❯ uptime
├── OUTPUT ──────────────────────────────────────────────────────────
 15:30:12 up 14 days,  3:18,  1 user,  load average: 0.08, 0.05, 0.01
└── [Elapsed: 12ms | Status: OK] ─────────────────────────────────────
```

For raw output suitable for shell scripts, pipelines, or `jq`:
```bash
rex exec <SERVER_IP>:7444 -t <TOKEN> -r "hostname"
```

> **Compound commands & pipes:** Wrap commands containing pipes (`|`), redirects (`>`), or chaining (`&&`) in quotes:
> ```bash
> rex exec <SERVER_IP>:7444 -t <TOKEN> "systemctl status nginx | head -n 15"
> ```

### Interactive Terminal Session (`rex connect`)

Attach an interactive shell session (supports Tab-completion, arrow keys, and full-screen editors):

```bash
rex connect <SERVER_IP>:7444 -t <TOKEN>
```

Alternatively, you can pass the address without explicit subcommands:
```bash
rex <SERVER_IP>:7444
```

### Remote System Telemetry (`rex info`)

Fetch hardware metrics (CPU cores, load average, available RAM, uptime) directly without spawning a shell:

```bash
rex info <SERVER_IP>:7444 -t <TOKEN>
```

### Local Node Management

When running on the server hosting `rex-node`:

```bash
rex status              # Check daemon service status
rex restart             # Restart the background daemon
rex logs                # Stream live journalctl service logs
rex update              # In-place update to the latest release
rex mode autonomous     # Switch security policy: autonomous | review | allowlist
```

---

## Python SDK Integration

Install the client package:

```bash
pip install git+https://github.com/dalroot/rex.git#subdirectory=rex-client
```

Usage example:

```python
import asyncio
from rex_client import RXPDirectClient

async def main():
    async with RXPDirectClient(host="<SERVER_IP>", token="YOUR_TOKEN", port=7444) as client:
        # Fast command execution
        result = await client.fast_exec("uname -a")
        print(f"Kernel: {result}")

        # Kernel hardware metrics
        info = await client.sysinfo()
        print(f"Memory available: {info['mem_available_kb']} KB")

if __name__ == "__main__":
    asyncio.run(main())
```

---

## Security Model

1. **Transport Encryption**: All socket communication enforces TLS 1.3. Plaintext connections are rejected by default.
2. **Bearer Token Authentication**: Each node requires an authentication token. Tokens can be provided via flag (`-t`), environment variable (`REX_TOKEN`), or prompted interactively without shell history echoing.
3. **Execution Policy Modes (`/etc/rex/allowlist.yaml`)**:
   - `autonomous`: Full execution freedom with safety filters against catastrophic system commands (e.g., `rm -rf /`, `mkfs`).
   - `review`: Mutating commands require explicit confirmation.
   - `allowlist`: Only explicitly listed commands are permitted.

---

## Repository Structure

```
├── rex-cli/              # Native Go client CLI (rex)
│   ├── main.go           # CLI entrypoint, argument parsing, output framing
│   ├── client.go         # TLS 1.3 socket transport & RXP binary framer
│   └── terminal.go       # PTY terminal emulator & raw mode handling
├── rex-node/             # Go daemon service (rex-node)
│   ├── server.go         # TLS TCP listener & HTTP probe server
│   ├── executor.go       # FastExec command runner & compound pipeline handler
│   ├── pty_runtime.go    # Linux PTY allocator & session manager
│   ├── allowlist.go      # Security mode policy enforcement
│   └── install.sh        # Server installer & systemd service setup
├── rex-client/           # Python SDK (rex_client)
├── install-cli.sh        # Client one-line installer for Linux/macOS
├── AGENT_GUIDE.md        # Reference manual for autonomous LLM agents
├── CHANGELOG.md          # Version history
└── LICENSE               # MIT License
```

---

## Contributing

Contributions, bug reports, and suggestions are welcome:
- Issues: [GitHub Issues](https://github.com/dalroot/rex/issues)
- Pull Requests: Feel free to submit PRs following standard Go and Python formatting standards.

---

## License

This project is open-source and licensed under the [MIT License](LICENSE).
