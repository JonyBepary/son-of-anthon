#!/data/data/com.termux/files/usr/bin/bash

# Setup script for building, configuring, and running son-of-anthon as a Termux background service

# Exit immediately if a command exits with a non-zero status
set -e

echo "==> Updating Termux packages..."
pkg update -y

echo "==> Installing dependencies (golang, make, termux-services)..."
pkg install golang make termux-services -y

echo "==> Building son-of-anthon..."
make build

echo "==> Moving binary to $PREFIX/bin..."
mv son-of-anthon "$PREFIX/bin/son-of-anthon"
chmod +x "$PREFIX/bin/son-of-anthon"

echo "==> Running interactive setup wizard..."
# The user needs to configure their API keys and settings
son-of-anthon setup

echo "==> Enabling services on boot..."
sv-enable

# Create the service directory
SERVICE_DIR="$PREFIX/var/service/son-of-anthon"
echo "==> Creating service directory at $SERVICE_DIR..."
mkdir -p "$SERVICE_DIR"
mkdir -p "$SERVICE_DIR/log"

# Create the run script
echo "==> Creating run script..."
cat << 'EOF' > "$SERVICE_DIR/run"
#!/data/data/com.termux/files/usr/bin/sh
exec 2>&1
export GODEBUG=netdns=go
exec son-of-anthon gateway
EOF

# Create the log run script
LOG_DIR="$PREFIX/var/log/son-of-anthon"
echo "==> Creating log script (logging to $LOG_DIR)..."
mkdir -p "$LOG_DIR"
cat << EOF > "$SERVICE_DIR/log/run"
#!/data/data/com.termux/files/usr/bin/sh
exec svlogd -tt "$LOG_DIR"
EOF

# Make scripts executable
chmod +x "$SERVICE_DIR/run"
chmod +x "$SERVICE_DIR/log/run"

echo ""
echo "✅ Installation and Setup Complete!"
echo "--------------------------------------------------------"
echo "To start the background daemon immediately, run:"
echo "  sv up son-of-anthon"
echo ""
echo "To check its status:"
echo "  sv status son-of-anthon"
echo ""
echo "To stop it:"
echo "  sv down son-of-anthon"
echo ""
echo "To view live logs:"
echo "  tail -f $LOG_DIR/current"
echo "--------------------------------------------------------"
