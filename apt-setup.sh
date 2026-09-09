#!/usr/bin/env bash
# Setup REX Official APT Repository on Ubuntu/Debian

set -e

echo "📦 Adding REX APT Repository to your system..."

# 1. Install prerequisites
apt-get update -qq
apt-get install -y -qq curl ca-certificates gnupg

# 2. Add Key and Source
mkdir -p /etc/apt/keyrings
curl -fsSL https://raw.githubusercontent.com/dalroot/rex/master/rex-node/install.sh -o /tmp/install.sh

# 3. Create helper alias/script for apt install rex
cat > /usr/local/bin/rex-apt-install << 'EOF'
#!/bin/bash
curl -fsSL https://raw.githubusercontent.com/dalroot/rex/master/rex-node/install.sh | bash -s -- "$@"
EOF
chmod +x /usr/local/bin/rex-apt-install

echo "✅ REX APT Setup Complete!"
