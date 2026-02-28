#!/bin/bash
# Son of Anthon - Linux systemd Setup
# Usage: sudo ./install-systemd.sh

set -e

echo "==> Son of Anthon - Linux systemd Setup"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "ERROR: Please run as root (sudo)"
    echo "Usage: sudo $0"
    exit 1
fi

# Find binary
if [ -f "./son-of-anthon-linux-amd64" ]; then
    BINARY_PATH="$(pwd)/son-of-anthon-linux-amd64"
elif [ -f "./son-of-anthon" ]; then
    BINARY_PATH="$(pwd)/son-of-anthon"
else
    echo "ERROR: No son-of-anthon binary found!"
    echo "Place son-of-anthon-linux-amd64 in current directory"
    exit 1
fi

echo "Found: $BINARY_PATH"

# Install binary
echo "Installing binary to /usr/local/bin..."
cp "$BINARY_PATH" /usr/local/bin/son-of-anthon
chmod +x /usr/local/bin/son-of-anthon

# Create user if doesn't exist
SON_USER=$(whoami)
SON_HOME=$HOME

# Create data directory
echo "Creating data directory..."
mkdir -p "$SON_HOME/.picoclaw/workspace"

# Copy config if not exists
if [ ! -f "$SON_HOME/.picoclaw/config.json" ]; then
    if [ -f "./config.example.json" ]; then
        echo "Copying config template..."
        cp ./config.example.json "$SON_HOME/.picoclaw/config.json"
    fi
fi

# Fix ownership
chown -R $SON_USER:$SON_USER "$SON_HOME/.picoclaw"

# Install systemd service
echo "Installing systemd service..."
cp ./install/son-of-anthon.service /etc/systemd/system/
systemctl daemon-reload

echo ""
echo "✅ Setup Complete!"
echo "============================================"
echo ""
echo "To enable on boot:"
echo "  sudo systemctl enable son-of-anthon"
echo ""
echo "To start now:"
echo "  sudo systemctl start son-of-anthon"
echo ""
echo "To check status:"
echo "  sudo systemctl status son-of-anthon"
echo ""
echo "To view logs:"
echo "  journalctl -u son-of-anthon -f"
echo "============================================"
