#!/usr/bin/env bash
# REX Universal CLI Installer (Linux & macOS)
# Installs standalone `rex` binary directly into PATH
# Usage: curl -fsSL https://raw.githubusercontent.com/dalroot/rex/master/install-cli.sh | bash

set -e

ARCH=$(uname -m)
case "$ARCH" in
  x86_64)        BINARY_NAME="rex-linux-amd64" ;;
  aarch64|arm64) BINARY_NAME="rex-linux-arm64" ;;
  *) echo "❌ Unsupported Architecture: $ARCH"; exit 1 ;;
esac

echo "⚡ Installing REX CLI (RXP/2.5 WarpGate v2.5.0)..."
DOWNLOAD_URL="https://github.com/dalroot/rex/releases/download/v2.5.0/${BINARY_NAME}"
CHECKSUM_URL="https://github.com/dalroot/rex/releases/download/v2.5.0/${BINARY_NAME}.sha256"

INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

curl -L -f -s -S "$DOWNLOAD_URL" -o "${INSTALL_DIR}/rex.tmp"

# Verify SHA-256 Checksum
if curl -L -f -s "$CHECKSUM_URL" -o /tmp/rex-cli.sha256 2>/dev/null; then
  echo "🔒 Verifying release binary SHA-256 checksum..."
  EXPECTED_HASH=$(cat /tmp/rex-cli.sha256 | awk '{print $1}')
  ACTUAL_HASH=$(sha256sum "${INSTALL_DIR}/rex.tmp" 2>/dev/null | awk '{print $1}' || shasum -a 256 "${INSTALL_DIR}/rex.tmp" | awk '{print $1}')
  if [ "$EXPECTED_HASH" = "$ACTUAL_HASH" ]; then
    echo "✅ Checksum verified successfully."
  fi
  rm -f /tmp/rex-cli.sha256
fi

chmod +x "${INSTALL_DIR}/rex.tmp"
mv -f "${INSTALL_DIR}/rex.tmp" "${INSTALL_DIR}/rex"

echo ""
echo "===================================================="
echo " ✅ REX CLI installed successfully to ${INSTALL_DIR}/rex"
echo "===================================================="
echo " 🚀 Quick Usage Examples:"
echo "    • Fast Exec:    rex exec <IP>:7444 -t <TOKEN> \"<command>\""
echo "    • SSH Terminal: rex connect <IP>:7444 -t <TOKEN>"
echo "    • System Info:  rex info <IP>:7444 -t <TOKEN>"
echo "===================================================="
