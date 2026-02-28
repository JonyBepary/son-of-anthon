#!/data/data/com.termux/files/usr/bin/bash
#
# Son of Anthon - Termux Installer
# Usage: ./install-termux.sh [--uninstall]
#

set -e

APP_NAME="son-of-anthon"
BINARY_NAME="$APP_NAME-termux"
SERVICE_NAME="$APP_NAME"
CONFIG_DIR="$HOME/.picoclaw"
SERVICE_DIR="$PREFIX/var/service/$SERVICE_NAME"

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
    sv down $SERVICE_NAME 2>/dev/null || true
    sv-disable $SERVICE_NAME 2>/dev/null || true
    
    # Remove files
    rm -f "$PREFIX/bin/$APP_NAME"
    rm -rf "$SERVICE_DIR"
    rm -rf "$CONFIG_DIR"
    
    log_info "Uninstall complete!"
    exit 0
}

find_binary() {
    local dir
    dir=$(dirname "$0")
    
    # Check current directory first
    if [ -f "$dir/son-of-anthon-termux" ]; then
        echo "$dir/son-of-anthon-termux"
    elif [ -f "$dir/son-of-anthon-linux-arm64" ]; then
        echo "$dir/son-of-anthon-linux-arm64"
    elif [ -f "$PWD/son-of-anthon-termux" ]; then
        echo "$PWD/son-of-anthon-termux"
    else
        log_error "No Termux binary found!"
        log_info "Expected: son-of-anthon-termux in current directory"
        exit 1
    fi
}

install_binary() {
    local src=$1
    log_info "Installing binary..."
    cp "$src" "$PREFIX/bin/$APP_NAME"
    chmod +x "$PREFIX/bin/$APP_NAME"
}

create_config() {
    log_info "Setting up config directory..."
    mkdir -p "$CONFIG_DIR/workspace"
    
    if [ ! -f "$CONFIG_DIR/config.json" ]; then
        log_info "Running setup wizard..."
        $APP_NAME setup
    fi
}

install_runit() {
    log_info "Installing runit service..."
    
    # Install termux-services if needed
    if ! command -v sv &> /dev/null; then
        log_info "Installing termux-services..."
        pkg install termux-services -y
    fi
    
    # Create service directory
    mkdir -p "$SERVICE_DIR/log"
    
    # Create run script
    cat > "$SERVICE_DIR/run" << 'EOF'
#!/data/data/com.termux/files/usr/bin/sh
exec 2>&1
export PATH="/data/data/com.termux/files/usr/bin:$PATH"
export GODEBUG=netdns=go
export HOME="/data/data/com.termux/files/home"
exec /data/data/com.termux/files/usr/bin/son-of-anthon gateway
EOF
    
    # Create log directory
    LOG_DIR="$CONFIG_DIR/termux-logs"
    mkdir -p "$LOG_DIR"
    
    cat > "$SERVICE_DIR/log/run" << EOF
#!/data/data/com.termux/files/usr/bin/sh
exec svlogd -tt "$LOG_DIR"
EOF
    
    chmod +x "$SERVICE_DIR/run"
    chmod +x "$SERVICE_DIR/log/run"
    
    # Enable service
    sv-enable $SERVICE_NAME 2>/dev/null || true
    
    log_info "runit service installed!"
}

# Main
main() {
    local binary
    
    if [ "$1" = "--uninstall" ]; then
        uninstall
    fi
    
    echo "============================================"
    echo "  Son of Anthon - Termux Installer"
    echo "============================================"
    echo ""
    
    # Find binary
    binary=$(find_binary)
    log_info "Using binary: $binary"
    
    # Install
    install_binary "$binary"
    create_config
    install_runit
    
    echo ""
    echo "============================================"
    log_info "Installation complete!"
    echo "============================================"
    echo ""
    echo "To start the daemon:"
    echo "  sv up $SERVICE_NAME"
    echo ""
    echo "To check status:"
    echo "  sv status $SERVICE_NAME"
    echo ""
    echo "To stop:"
    echo "  sv down $SERVICE_NAME"
    echo ""
    echo "To view logs:"
    echo "  tail -f $CONFIG_DIR/termux-logs/current"
    echo ""
    echo "To run manually:"
    echo "  $APP_NAME gateway"
    echo ""
}

main "$@"
