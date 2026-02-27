#!/data/data/com.termux/files/usr/bin/bash

# Setup script for configuring son-of-anthon as a Termux background service

# 1. Install required packages
echo "==> Installing termux-services..."
pkg install termux-services -y

# 2. Enable services to start automatically on Termux boot
echo "==> Enabling services on boot..."
sv-enable

# 3. Create the service directory
SERVICE_DIR="$PREFIX/var/service/son-of-anthon"
echo "==> Creating service directory at $SERVICE_DIR..."
mkdir -p "$SERVICE_DIR"
mkdir -p "$SERVICE_DIR/log"

# 4. Create the run script
echo "==> Creating run script..."
cat << 'EOF' > "$SERVICE_DIR/run"
#!/data/data/com.termux/files/usr/bin/sh
exec 2>&1
export GODEBUG=netdns=go
cd ~/pico-son-of-anthon
exec ./son-of-anthon-termux gateway
EOF

# 5. Create the log run script
echo "==> Creating log script..."
mkdir -p ~/pico-son-of-anthon/termux-logs
cat << 'EOF' > "$SERVICE_DIR/log/run"
#!/data/data/com.termux/files/usr/bin/sh
exec svlogd -tt ~/pico-son-of-anthon/termux-logs
EOF

# Make scripts executable
chmod +x "$SERVICE_DIR/run"
chmod +x "$SERVICE_DIR/log/run"

echo ""
echo "✅ Setup Complete!"
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
echo "  tail -f ~/pico-son-of-anthon/termux-logs/current"
echo "--------------------------------------------------------"
