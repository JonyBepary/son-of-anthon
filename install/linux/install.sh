#!/bin/bash
#
# Son of Anthon - Linux Installer
# Supports: Ubuntu/Debian, Fedora/RHEL, Arch
# Usage: sudo ./install.sh [--uninstall]
#

set -e

APP_NAME="son-of-anthon"
BINARY_NAME="son-of-anthon"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="$HOME/.picoclaw"
SYSTEMD_DIR="/etc/systemd/system"
INIT_DIR="/etc/init.d"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
    elif [ -f /etc/lsb-release ]; then
        . /etc/lsb-release
        OS=$DISTRIB_ID
    else
        OS="unknown"
    fi
    echo "$OS"
}

uninstall() {
    log_info "Uninstalling $APP_NAME..."
    
    # Stop service
    if command -v systemctl &> /dev/null; then
        systemctl stop $APP_NAME 2>/dev/null || true
        systemctl disable $APP_NAME 2>/dev/null || true
        rm -f $SYSTEMD_DIR/$APP_NAME.service
        systemctl daemon-reload
    fi
    
    # Remove binary
    rm -f $INSTALL_DIR/$BINARY_NAME
    
    # Keep config (optional)
    # rm -rf $CONFIG_DIR
    
    log_info "Uninstall complete!"
    exit 0
}

# Check if running as root for install
check_root() {
    if [ "$EUID" -ne 0 ] && [ "$1" != "--uninstall" ]; then
        log_error "Please run as root: sudo $0"
        exit 1
    fi
}

# Find binary
find_binary() {
    local dir
    dir=$(dirname "$0")
    
    if [ -f "$dir/son-of-anthon-linux-amd64" ]; then
        echo "$dir/son-of-anthon-linux-amd64"
    elif [ -f "$dir/son-of-anthon" ]; then
        echo "$dir/son-of-anthon"
    else
        log_error "No binary found in current directory!"
        log_info "Download from: https://github.com/JonyBepary/son-of-anthon/releases"
        exit 1
    fi
}

# Install binary
install_binary() {
    local src=$1
    log_info "Installing binary to $INSTALL_DIR..."
    cp "$src" "$INSTALL_DIR/$BINARY_NAME"
    chmod +x "$INSTALL_DIR/$BINARY_NAME"
}

# Create config
create_config() {
    log_info "Creating config directory..."
    mkdir -p "$CONFIG_DIR/workspace"
    
    if [ ! -f "$CONFIG_DIR/config.json" ]; then
        log_info "Running initial setup..."
        $INSTALL_DIR/$BINARY_NAME setup
    fi
}

# Install systemd service
install_systemd() {
    if ! command -v systemctl &> /dev/null; then
        log_warn "systemd not found, skipping service installation"
        return
    fi
    
    log_info "Installing systemd service..."
    
    cat > $SYSTEMD_DIR/$APP_NAME.service << EOF
[Unit]
Description=Son of Anthon - Multi-agent AI Assistant
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SUDO_USER
WorkingDirectory=$CONFIG_DIR
ExecStart=$INSTALL_DIR/$BINARY_NAME gateway
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal
Environment=HOME=$HOME
Environment=PERSONAL_OS_CONFIG=$CONFIG_DIR/config.json

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    systemctl enable $APP_NAME
    
    log_info "systemd service installed!"
}

# Install init.d script (for SysV init)
install_initd() {
    if [ -d "$INIT_DIR" ] && ! command -v systemctl &> /dev/null; then
        log_info "Installing init.d script..."
        
        cat > $INIT_DIR/$APP_NAME << 'EOFSCRIPT'
#!/bin/bash
### BEGIN INIT INFO
# Provides:          son-of-anthon
# Required-Start:     $network $remote_fs $syslog
# Required-Stop:      $network $remote_fs $syslog
# Default-Start:      2 3 4 5
# Default-Stop:       0 1 6
# Description:        Son of Anthon AI Assistant
### END INIT INFO

APP_NAME=son-of-anthon
BINARY=/usr/local/bin/$APP_NAME
CONFIG=$HOME/.picoclaw/config.json
PIDFILE=/var/run/$APP_NAME.pid

start() {
    if [ -f $PIDFILE ]; then
        echo "$APP_NAME already running"
        return 1
    fi
    echo "Starting $APP_NAME..."
    cd $HOME/.picoclaw
    nohup $BINARY gateway > /var/log/$APP_NAME.log 2>&1 &
    echo $! > $PIDFILE
}

stop() {
    if [ ! -f $PIDFILE ]; then
        echo "$APP_NAME not running"
        return 1
    fi
    echo "Stopping $APP_NAME..."
    kill $(cat $PIDFILE)
    rm -f $PIDFILE
}

case "$1" in
    start) start ;;
    stop) stop ;;
    restart) stop; sleep 2; start ;;
    status) 
        if [ -f $PIDFILE ]; then
            echo "$APP_NAME running (PID: $(cat $PIDFILE))"
        else
            echo "$APP_NAME not running"
        fi
        ;;
    *) echo "Usage: $0 {start|stop|restart|status}"; exit 1 ;;
esac
EOFSCRIPT
        
        chmod +x $INIT_DIR/$APP_NAME
        update-rc.d $APP_NAME defaults 2>/dev/null || true
    fi
}

# Main
main() {
    local binary
    
    # Check for uninstall
    if [ "$1" = "--uninstall" ]; then
        check_root --uninstall
        uninstall
    fi
    
    check_root
    
    echo "============================================"
    echo "  Son of Anthon - Linux Installer"
    echo "============================================"
    echo ""
    
    OS=$(detect_os)
    log_info "Detected OS: $OS"
    
    # Find binary
    binary=$(find_binary)
    log_info "Using binary: $binary"
    
    # Install
    install_binary "$binary"
    create_config
    install_systemd
    install_initd
    
    echo ""
    echo "============================================"
    log_info "Installation complete!"
    echo "============================================"
    echo ""
    echo "To start the service:"
    echo "  sudo systemctl start $APP_NAME"
    echo ""
    echo "To enable on boot:"
    echo "  sudo systemctl enable $APP_NAME"
    echo ""
    echo "To check status:"
    echo "  sudo systemctl status $APP_NAME"
    echo ""
    echo "To run manually:"
    echo "  $BINARY_NAME gateway"
    echo ""
}

main "$@"
