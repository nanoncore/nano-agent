#!/bin/bash
# Nanoncore Edge Agent Installation Script
# Usage: curl -sSL https://get.nanoncore.com/agent | sudo bash

set -e

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/nano-agent"
LOG_DIR="/var/log/nano-agent"
SYSTEMD_DIR="/etc/systemd/system"

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64)
        ARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$OS" != "linux" ]; then
    echo "Error: nano-agent only supports Linux"
    exit 1
fi

echo "Nanoncore Edge Agent Installer"
echo "=============================="
echo ""
echo "Architecture: $ARCH"
echo "OS: $OS"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Error: This script must be run as root"
    echo "Try: sudo $0"
    exit 1
fi

# Create directories
echo "Creating directories..."
mkdir -p "$CONFIG_DIR"
mkdir -p "$LOG_DIR"
chmod 750 "$CONFIG_DIR"
chmod 755 "$LOG_DIR"

# Download latest binary
# In production, this would download from a release server
BINARY_URL="https://github.com/nanoncore/nano-agent/releases/latest/download/nano-agent-${OS}-${ARCH}"
echo "Downloading nano-agent..."

# For now, copy from local build if available
if [ -f "/tmp/nano-agent" ]; then
    echo "Using local build..."
    cp /tmp/nano-agent "$INSTALL_DIR/nano-agent"
else
    # Try to download (will fail if releases aren't published yet)
    if command -v curl &> /dev/null; then
        curl -sSL -o "$INSTALL_DIR/nano-agent" "$BINARY_URL" || {
            echo "Warning: Could not download from $BINARY_URL"
            echo "Please build and install manually:"
            echo "  go build -o /usr/local/bin/nano-agent ./cmd/nano-agent"
            exit 1
        }
    elif command -v wget &> /dev/null; then
        wget -q -O "$INSTALL_DIR/nano-agent" "$BINARY_URL" || {
            echo "Warning: Could not download from $BINARY_URL"
            exit 1
        }
    else
        echo "Error: curl or wget required"
        exit 1
    fi
fi

chmod +x "$INSTALL_DIR/nano-agent"

# Verify installation
echo "Verifying installation..."
"$INSTALL_DIR/nano-agent" version

# Install systemd service
echo "Installing systemd service..."
cat > "$SYSTEMD_DIR/nano-agent.service" << 'EOF'
[Unit]
Description=Nanoncore Edge Agent
Documentation=https://docs.nanoncore.com/agent
After=network-online.target
Wants=network-online.target
After=vpp.service
Wants=vpp.service

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/nano-agent run --heartbeat-interval 30s --config-sync-interval 5m
Restart=always
RestartSec=10
WorkingDirectory=/etc/nano-agent
Environment="PATH=/usr/local/bin:/usr/bin:/bin"
StandardOutput=journal
StandardError=journal
SyslogIdentifier=nano-agent
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/etc/nano-agent /var/log/nano-agent
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload

echo ""
echo "Installation complete!"
echo ""
echo "Next steps:"
echo "  1. Enroll this node:"
echo "     sudo nano-agent enroll \\"
echo "       --api https://api.nanoncore.com \\"
echo "       --token YOUR_TOKEN \\"
echo "       --node-id YOUR_NODE_ID"
echo ""
echo "  2. Start the agent:"
echo "     sudo systemctl enable nano-agent"
echo "     sudo systemctl start nano-agent"
echo ""
echo "  3. Check status:"
echo "     sudo nano-agent status"
echo "     sudo journalctl -u nano-agent -f"
