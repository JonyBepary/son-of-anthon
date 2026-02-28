#!/data/data/com.termux/files/usr/bin/bash

# Setup script for running son-of-anthon as a Termux background service
# Usage: ./setup-termux.sh [binary_path]
#   If no binary_path provided, assumes son-of-anthon-termux exists in current dir

set -e

BINARY_NAME="son-of-anthon"
SERVICE_NAME="son-of-anthon"

echo "==> Son of Anthon - Termux Service Setup"
echo ""

# Determine binary path
if [ -n "$1" ]; then
    BINARY_PATH="$1"
elif [ -f "./son-of-anthon-termux" ]; then
    BINARY_PATH="$(pwd)/son-of-anthon-termux"
elif [ -f "./son-of-anthon" ]; then
    BINARY_PATH="$(pwd)/son-of-anthon"
else
    echo "ERROR: No binary found!"
    echo "Usage: $0 [path_to_son_of_anthon_binary]"
    echo ""
    echo "Place son-of-anthon-termux in current directory or provide path as argument"
    exit 1
fi

echo "==> Using binary: $BINARY_PATH"

# Check binary exists and is executable
if [ ! -f "$BINARY_PATH" ]; then
    echo "ERROR: Binary not found at $BINARY_PATH"
    exit 1
fi

# Install termux-services if not present
if ! command -v sv &> /dev/null; then
    echo "==> Installing termux-services..."
    pkg install termux-services -y
fi

# Ensure $PREFIX/bin exists in PATH
export PATH="$PREFIX/bin:$PATH"

# Copy binary to PREFIX/bin
echo "==> Installing binary to $PREFIX/bin..."
cp "$BINARY_PATH" "$PREFIX/bin/$BINARY_NAME"
chmod +x "$PREFIX/bin/$BINARY_NAME"

# Check if setup is needed
CONFIG_PATH="$HOME/.picoclaw/config.json"
if [ ! -f "$CONFIG_PATH" ]; then
    echo "==> Running initial setup wizard..."
    echo "    (Configure your API keys when prompted)"
    $BINARY_NAME setup
fi

# Create service directory
SERVICE_DIR="$PREFIX/var/service/$SERVICE_NAME"
echo "==> Creating service at $SERVICE_DIR..."
mkdir -p "$SERVICE_DIR"
mkdir -p "$SERVICE_DIR/log"

# Create the run script with proper PATH
cat << 'EOF' > "$SERVICE_DIR/run"
#!/data/data/com.termux/files/usr/bin/sh
exec 2>&1
export PATH="/data/data/com.termux/files/usr/bin:$PATH"
export GODEBUG=netdns=go
export HOME="/data/data/com.termux/files/home"
exec /data/data/com.termux/files/usr/bin/son-of-anthon gateway
EOF

# Create log directory
LOG_DIR="$HOME/.picoclaw/termux-logs"
mkdir -p "$LOG_DIR"

# Create log run script
cat << EOF > "$SERVICE_DIR/log/run"
#!/data/data/com.termux/files/usr/bin/sh
exec svlogd -tt "$LOG_DIR"
EOF

# Make scripts executable
chmod +x "$SERVICE_DIR/run"
chmod +x "$SERVICE_DIR/log/run"

# Enable service
echo "==> Enabling service on boot..."
sv-enable $SERVICE_NAME 2>/dev/null || true

echo ""
echo "✅ Setup Complete!"
echo "--------------------------------------------------------"
echo "To start the daemon now:"
echo "  sv up $SERVICE_NAME"
echo ""
echo "To check status:"
echo "  sv status $SERVICE_NAME"
echo ""
echo "To stop:"
echo "  sv down $SERVICE_NAME"
echo ""
echo "To view logs:"
echo "  tail -f $LOG_DIR/current"
echo ""
echo "To run manually:"
echo "  son-of-anthon gateway"
echo "--------------------------------------------------------"
