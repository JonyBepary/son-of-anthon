#!/bin/bash
#
# Son of Anthon - macOS Installer
# Usage: sudo ./install-darwin.sh [--uninstall]
#

set -e

APP_NAME="son-of-anthon"
BINARY_NAME="son-of-anthon-darwin"
CONFIG_DIR="$HOME/.picoclaw"
LAUNCHD_DIR="$HOME/Library/LaunchAgents"
PLIST_NAME="com.sonofanthon.gateway.plist"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

uninstall() {
    log_info "Uninstalling $APP_NAME..."
    
    # Stop service
    launchctl unload "$LAUNCHD_DIR/$PLIST_NAME" 2>/dev/null || true
    
    # Remove files
    rm -f /usr/local/bin/$APP_NAME
    rm -f "$LAUNCHD_DIR/$PLIST_NAME"
    
    # Keep config
    # rm -rf "$CONFIG_DIR"
    
    log_info "Uninstall complete!"
    exit 0
}

find_binary() {
    local dir
    dir=$(dirname "$0")
    
    # Check for darwin binary
    if [ -f "$dir/son-of-anthon-darwin-amd64" ]; then
        echo "$dir/son-of-anthon-darwin-amd64"
    elif [ -f "$dir/son-of-anthon-darwin-arm64" ]; then
        echo "$dir/son-of-anthon-darwin-arm64"
    elif [ -f "$dir/son-of-anthon" ]; then
        echo "$dir/son-of-anthon"
    else
        log_error "No macOS binary found!"
        log_info "Expected: son-of-anthon-darwin-* in current directory"
        exit 1
    fi
}

install_binary() {
    local src=$1
    log_info "Installing binary..."
    
    # Check if /usr/local exists and is writable
    if [ -d /usr/local ] && [ -w /usr/local/bin ]; then
        cp "$src" /usr/local/bin/$APP_NAME
        chmod +x /usr/local/bin/$APP_NAME
    else
        # Use homebrew prefix
        BREW_PREFIX=$(brew --prefix 2>/dev/null || echo "/opt/homebrew")
        cp "$src" "$BREW_PREFIX/bin/$APP_NAME"
        chmod +x "$BREW_PREFIX/bin/$APP_NAME"
    fi
}

create_config() {
    log_info "Setting up config directory..."
    mkdir -p "$CONFIG_DIR/workspace"
    
    if [ ! -f "$CONFIG_DIR/config.json" ]; then
        log_info "Running setup wizard..."
        $APP_NAME setup
    fi
}

install_launchd() {
    log_info "Installing launchd service..."
    
    mkdir -p "$LAUNCHD_DIR"
    
    # Get binary path
    if [ -x /usr/local/bin/$APP_NAME ]; then
        BINARY_PATH="/usr/local/bin/$APP_NAME"
    else
        BREW_PREFIX=$(brew --prefix 2>/dev/null || echo "/opt/homebrew")
        BINARY_PATH="$BREW_PREFIX/bin/$APP_NAME"
    fi
    
    cat > "$LAUNCHD_DIR/$PLIST_NAME" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.sonofanthon.gateway</string>
    <key>ProgramArguments</key>
    <array>
        <string>$BINARY_PATH</string>
        <string>gateway</string>
    </array>
    <key>WorkingDirectory</key>
    <string>$CONFIG_DIR</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$CONFIG_DIR/logs/gateway.log</string>
    <key>StandardErrorPath</key>
    <string>$CONFIG_DIR/logs/gateway.error.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>HOME</key>
        <string>$HOME</string>
        <key>PERSONAL_OS_CONFIG</key>
        <string>$CONFIG_DIR/config.json</string>
    </dict>
</dict>
</plist>
EOF
    
    # Load service
    launchctl load "$LAUNCHD_DIR/$PLIST_NAME"
    
    log_info "launchd service installed!"
}

# Main
main() {
    local binary
    
    if [ "$1" = "--uninstall" ]; then
        uninstall
    fi
    
    echo "============================================"
    echo "  Son of Anthon - macOS Installer"
    echo "============================================"
    echo ""
    
    # Find binary
    binary=$(find_binary)
    log_info "Using binary: $binary"
    
    # Check for Homebrew (optional)
    if ! command -v brew &> /dev/null; then
        log_warn "Homebrew not found. Binary will be installed to /usr/local/bin"
        log_warn "For better management, install Homebrew: https://brew.sh"
    fi
    
    # Install
    install_binary "$binary"
    create_config
    install_launchd
    
    echo ""
    echo "============================================"
    log_info "Installation complete!"
    echo "============================================"
    echo ""
    echo "To start the service:"
    echo "  launchctl load $LAUNCHD_DIR/$PLIST_NAME"
    echo "  # or"
    echo "  launchctl start com.sonofanthon.gateway"
    echo ""
    echo "To stop:"
    echo "  launchctl stop com.sonofanthon.gateway"
    echo ""
    echo "To view logs:"
    echo "  tail -f $CONFIG_DIR/logs/gateway.log"
    echo ""
    echo "To run manually:"
    echo "  $APP_NAME gateway"
    echo ""
}

main "$@"
