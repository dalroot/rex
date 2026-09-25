#!/usr/bin/env bash
# REX One-Line Super Installer (Linux / Ubuntu / Debian / CentOS)
# Usage: curl -fsSL https://raw.githubusercontent.com/dalroot/rex/master/rex-node/install.sh | bash

set -e

TOKEN=""
PORT="7443"
TCP_PORT="7444"
MODE="autonomous"

# Parse optional args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --token)    TOKEN="$2";    shift 2 ;;
    --port)     PORT="$2";     shift 2 ;;
    --tcp-port) TCP_PORT="$2"; shift 2 ;;
    --mode)     MODE="$2";     shift 2 ;;
    *) shift ;;
  esac
done

if [[ -z "$TOKEN" ]] && [[ -f /etc/rex/config.yaml ]]; then
  TOKEN=$(grep "token:" /etc/rex/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"')
fi

if [[ -z "$TOKEN" ]]; then
  TOKEN=$(openssl rand -hex 16 2>/dev/null || date +%s | md5sum | head -c 32)
fi

echo "⚡ Installing REX Node Daemon (RXP/2.5 v2.5.0)..."

# Check architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)
    BINARY_NAME="rex-node-linux-amd64"
    CLI_BINARY_NAME="rex-linux-amd64"
    ;;
  aarch64|arm64)
    BINARY_NAME="rex-node-linux-arm64"
    CLI_BINARY_NAME="rex-linux-arm64"
    ;;
  *) echo "Unsupported Architecture: $ARCH"; exit 1 ;;
esac

# Detect public inbound IP address: prioritize interface IP, then fallback to diverse reflectors
echo "🔍 Detecting public server IP address..."
INTERFACE_IP=$(ip -4 addr show scope global 2>/dev/null | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | grep -vE '^(10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[0-1])\.|127\.|169\.254\.)' | head -n 1)

if [ -n "$INTERFACE_IP" ]; then
  SERVER_IP="$INTERFACE_IP"
  echo "✅ Inbound Public IP detected directly on network interface: ${SERVER_IP}"
else
  for REFLECTOR in \
    "https://api4.ipify.org" \
    "https://icanhazip.com" \
    "https://ifconfig.io/ip" \
    "https://1.1.1.1/cdn-cgi/trace"
  do
    DETECTED=$(curl -s -4 --connect-timeout 2 "$REFLECTOR" 2>/dev/null | grep -oE '([0-9]{1,3}\.){3}[0-9]{1,3}' | head -n 1)
    if [ -n "$DETECTED" ]; then
      SERVER_IP="$DETECTED"
      break
    fi
  done
  SERVER_IP="${SERVER_IP:-127.0.0.1}"
  echo "✅ Detected Public IP: ${SERVER_IP}"
fi

mkdir -p /usr/local/bin /etc/rex

# Stop existing running daemon if updating
systemctl stop rex-node 2>/dev/null || true
pkill -f rex-node 2>/dev/null || true
rm -f /usr/local/bin/rex-node.tmp

# Download pre-compiled binary safely via GitHub Release redirects
DOWNLOAD_URL="https://github.com/dalroot/rex/releases/download/v2.5.0/${BINARY_NAME}"
CHECKSUM_URL="https://github.com/dalroot/rex/releases/download/v2.5.0/${BINARY_NAME}.sha256"

curl -L -f -s -S "$DOWNLOAD_URL" -o /usr/local/bin/rex-node.tmp

# Verify SHA-256 Checksum if available
if curl -L -f -s "$CHECKSUM_URL" -o /tmp/rex-node.sha256 2>/dev/null; then
  echo "🔒 Verifying release binary SHA-256 checksum..."
  EXPECTED_HASH=$(cat /tmp/rex-node.sha256 | awk '{print $1}')
  ACTUAL_HASH=$(sha256sum /usr/local/bin/rex-node.tmp | awk '{print $1}')
  if [ "$EXPECTED_HASH" != "$ACTUAL_HASH" ]; then
    echo "❌ Integrity Error: SHA-256 Checksum verification failed!"
    rm -f /usr/local/bin/rex-node.tmp /tmp/rex-node.sha256
    exit 1
  fi
  echo "✅ Binary Checksum Verified."
  rm -f /tmp/rex-node.sha256
fi

chmod +x /usr/local/bin/rex-node.tmp
mv -f /usr/local/bin/rex-node.tmp /usr/local/bin/rex-node

# Generate Self-Signed TLS 1.3 Certificate if missing
if [ ! -f /etc/rex/cert.pem ] || [ ! -f /etc/rex/key.pem ]; then
  echo "🔐 Generating Self-Signed TLS 1.3 Certificate..."
  openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout /etc/rex/key.pem \
    -out /etc/rex/cert.pem \
    -days 3650 \
    -subj "/CN=rex-node" 2>/dev/null || true
fi

# Write Config
cat > /etc/rex/config.yaml << EOF
token: "${TOKEN}"
port: ${PORT}
tcp_port: ${TCP_PORT}
tls: true
cert_file: /etc/rex/cert.pem
key_file: /etc/rex/key.pem
allowlist: /etc/rex/allowlist.yaml
log_level: info
EOF

# Write Default Allowlist
cat > /etc/rex/allowlist.yaml << EOF
mode: "${MODE}"
denied_commands:
  - "rm -rf /"
  - "chmod -R 777 /"
  - "mkfs"
EOF

# Setup CLI tool `/usr/local/bin/rex`
echo "📦 Installing native REX CLI into /usr/local/bin/rex..."
CLI_DOWNLOAD_URL="https://github.com/dalroot/rex/releases/download/v2.5.0/${CLI_BINARY_NAME}"
if curl -L -f -s -S "$CLI_DOWNLOAD_URL" -o /usr/local/bin/rex.tmp 2>/dev/null; then
  chmod +x /usr/local/bin/rex.tmp
  mv -f /usr/local/bin/rex.tmp /usr/local/bin/rex
else
cat > /usr/local/bin/rex << 'EOF'
#!/bin/bash
INTERFACE_IP=$(ip -4 addr show scope global 2>/dev/null | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | grep -vE '^(10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[0-1])\.|127\.|169\.254\.)' | head -n 1)
SERVER_IP="${INTERFACE_IP:-$(curl -s -4 --connect-timeout 2 https://1.1.1.1/cdn-cgi/trace 2>/dev/null | awk -F= '/ip/{print $2}' | grep -oE '([0-9]{1,3}\.){3}[0-9]{1,3}' || curl -s -4 --connect-timeout 2 https://api4.ipify.org 2>/dev/null | grep -oE '([0-9]{1,3}\.){3}[0-9]{1,3}' || hostname -I 2>/dev/null | awk '{print $1}' || echo "127.0.0.1")}"
CONFIG_FILE="/etc/rex/config.yaml"
ALLOWLIST_FILE="/etc/rex/allowlist.yaml"

case "$1" in
  status)
    systemctl status rex-node --no-pager
    ;;
  start)
    systemctl start rex-node
    echo "✅ REX Node started."
    ;;
  stop)
    systemctl stop rex-node
    echo "🛑 REX Node stopped."
    ;;
  restart)
    systemctl restart rex-node
    echo "🔄 REX Node restarted."
    ;;
  logs)
    journalctl -u rex-node -f
    ;;
  mode)
    if [ -n "$2" ]; then
      if [[ "$2" == "autonomous" || "$2" == "review" || "$2" == "allowlist" ]]; then
        sed -i "s/^mode:.*/mode: \"$2\"/" "$ALLOWLIST_FILE"
        systemctl restart rex-node
        echo "🛡️ REX Security Mode updated to: $2 (Daemon restarted)"
      else
        echo "❌ Invalid mode. Choose from: autonomous | review | allowlist"
      fi
    else
      CURRENT_MODE=$(grep "^mode:" "$ALLOWLIST_FILE" | awk '{print $2}' | tr -d '"')
      echo "🛡️ Current REX Security Mode: ${CURRENT_MODE:-autonomous}"
    fi
    ;;
  config)
    echo "--- Config ($CONFIG_FILE) ---"
    cat "$CONFIG_FILE"
    echo ""
    echo "--- Security Policy ($ALLOWLIST_FILE) ---"
    cat "$ALLOWLIST_FILE"
    ;;
  token)
    grep "token:" "$CONFIG_FILE" | awk '{print $2}' | tr -d '"'
    ;;
  ip)
    echo "$SERVER_IP"
    ;;
  info)
    TOKEN=$(grep "token:" "$CONFIG_FILE" | awk '{print $2}' | tr -d '"')
    MODE=$(grep "^mode:" "$ALLOWLIST_FILE" | awk '{print $2}' | tr -d '"')
    echo "=========================================="
    echo " 📍 REX Node Status & Info (RXP/2.5 WarpGate)"
    echo "=========================================="
    echo " Server IP: $SERVER_IP"
    echo " RXP TCP:   7444"
    echo " Web Port:  7443"
    echo " Mode:      ${MODE:-autonomous}"
    echo " Token:     $TOKEN"
    echo "=========================================="
    ;;
  update)
    echo "🔄 Updating REX Node daemon to latest version..."
    ARCH=$(uname -m)
    case "$ARCH" in
      x86_64)        BINARY_NAME="rex-node-linux-amd64" ;;
      aarch64|arm64) BINARY_NAME="rex-node-linux-arm64" ;;
      *) echo "❌ Unsupported Architecture: $ARCH"; exit 1 ;;
    esac
    curl -L -f -s -S "https://github.com/dalroot/rex/releases/download/v2.5.0/${BINARY_NAME}" -o /usr/local/bin/rex-node.tmp
    chmod +x /usr/local/bin/rex-node.tmp
    mv -f /usr/local/bin/rex-node.tmp /usr/local/bin/rex-node
    systemctl restart rex-node
    echo "✅ REX Node daemon successfully updated and restarted."
    ;;
  uninstall|remove)
    echo "⚠️ Removing REX Node from server..."
    systemctl stop rex-node 2>/dev/null || true
    systemctl disable rex-node 2>/dev/null || true
    rm -f /etc/systemd/system/rex-node.service
    systemctl daemon-reload
    rm -rf /usr/local/bin/rex-node /usr/local/bin/rex /etc/rex
    echo "🗑️ REX Node has been completely uninstalled and removed."
    ;;
  *)
    echo "REX CLI Management Tool (v1.0.1)"
    echo "Usage:"
    echo "  rex mode {autonomous|review|allowlist}  Get/Set security mode"
    echo "  rex info                                Display IP, port, mode & token"
    echo "  rex status                              Check daemon status"
    echo "  rex restart                             Restart daemon"
    echo "  rex update                              Update daemon to latest release"
    echo "  rex logs                                Stream live logs"
    echo "  rex uninstall                           Remove REX completely"
    ;;
esac
EOF
chmod +x /usr/local/bin/rex
fi

# Setup Systemd Service
cat > /etc/systemd/system/rex-node.service << EOF
[Unit]
Description=REX Node Daemon
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/rex-node --config /etc/rex/config.yaml
Restart=always

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now rex-node

# Open firewall ports 7443 and 7444 if ufw or iptables exist
ufw allow 7443/tcp 2>/dev/null || iptables -A INPUT -p tcp --dport 7443 -j ACCEPT 2>/dev/null || true
ufw allow 7444/tcp 2>/dev/null || iptables -A INPUT -p tcp --dport 7444 -j ACCEPT 2>/dev/null || true

echo ""
echo "===================================================="
echo " ✅ REX Node installed and running successfully!"
echo "===================================================="
echo " ├─ Server IP: ${SERVER_IP}"
echo " ├─ RXP TCP:   ${TCP_PORT}"
echo " ├─ Web Port:  ${PORT}"
echo " ├─ Mode:      ${MODE}"
echo " └─ Token:     ${TOKEN}"
echo "===================================================="
echo " 💡 CLI Commands:"
echo "    • rex mode autonomous   (Set mode: autonomous | review | allowlist)"
echo "    • rex info              (Print IP, Port & Token)"
echo "    • rex logs              (Stream live logs)"
echo "    • rex uninstall         (Remove REX)"
echo "===================================================="
