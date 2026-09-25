# 🤖 REX Protocol — Autonomous AI Agent Operational Manual

You are interacting with a remote Linux server running **REX (Remote EXecution Protocol RXP/2.5)**.
REX is an **Agent-Native Direct Infrastructure Runtime**, **NOT** a legacy SSH daemon or a REST API.

---

## ⚡ Golden Operational Rules for AI Agents

1. **NEVER execute `ssh` or `sshpass`:** Port 22 is disabled or firewalled.
2. **NEVER probe HTTP REST endpoints:** REX does not expose `/api/status`, `/exec`, or web forms.
3. **NEVER write temporary scripts on the local host** to execute remote tasks.
4. **ALWAYS use the local `rex` binary directly.**

---

## 🚀 Standard Command Execution

### 1. Fast Batch Execution (`rex exec`)
Use this for non-interactive commands (inspecting services, querying SQLite, checking logs):

```bash
rex exec <SERVER_IP>:7444 -t <TOKEN> "<command>"
```

> 💡 **Best Practice for Pipes and Compound Commands:**
> If your command contains pipes (`|`), redirects (`>`), or chaining (`&&`, `;`), wrap it cleanly in `bash -c`:
> ```bash
> rex exec <SERVER_IP>:7444 -t <TOKEN> "bash -c 'systemctl status x-ui | head -n 30'"
> ```

### 2. Live Interactive Shell (`rex connect`)
Use this when you need an interactive pseudo-terminal session:

```bash
rex connect <SERVER_IP>:7444 -t <TOKEN>
```

### 3. Hardware & Diagnostic Metrics (`rex info`)
Fetches instant kernel metrics (RAM, CPUs, Load Average, Uptime) without spawning a shell:

```bash
rex info <SERVER_IP>:7444 -t <TOKEN>
```

---

## 🛠️ Common Errors & Self-Correction

| Error Seen | Cause | Immediate Agent Fix |
| :--- | :--- | :--- |
| `unexpected argument` | Shell operator unquoted | Wrap compound command in `bash -c '...'` |
| `Authentication token required` | Token missing | Pass `-t <TOKEN>` or `export REX_TOKEN=<TOKEN>` |
| `Connection failed: context deadline` | Blocked port or wrong IP | Verify port is `7444` and IP is the inbound server IP |
| `handshake read failed: EOF` | Server runs TLS 1.3 | Omit `--tls=false` (TLS 1.3 is default) |
| `404 page not found` | Attempted cURL to port 7443 | Stop using cURL; use `rex exec <IP>:7444` |

---

## 📦 Universal CLI Installation (If `rex` is not found)

```bash
curl -fsSL https://raw.githubusercontent.com/dalroot/rex/master/install-cli.sh | bash
```
