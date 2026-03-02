#!/data/data/com.termux/files/usr/bin/bash
#
# Son of Anthon - Onboard Command
# One-command setup: installs deps, configures, and starts services
#

set -e

APP_NAME="son-of-anthon"
SERVICE_NAME="$APP_NAME"
CONFIG_DIR="$HOME/.picoclaw"
SERVICE_DIR="$PREFIX/var/service/$SERVICE_NAME"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_step() { echo -e "${BLUE}[STEP]${NC} $1"; }

check_proot() {
    if [ -f /usr/bin/apt ] || [ -f /bin/apt ]; then
        return 0
    fi
    return 1
}

install_deps() {
    log_step "Installing dependencies..."
    
    pkg update -y
    
    # Core dependencies
    pkg install -y coreutils git curl wget
    
    # For runit (service manager)
    pkg install -y termux-services
    
    # For proot-distro (optional)
    if [ "$1" = "--debian" ] || [ "$1" = "--deb" ]; then
        pkg install -y proot-distro
    fi
    
    # For voice (optional)
    pkg install -y sox ffmpeg
    
    log_info "Dependencies installed!"
}

install_proot_debian() {
    log_step "Installing Proot Debian..."
    
    if ! command -v proot-distro &> /dev/null; then
        pkg install -y proot-distro
    fi
    
    # Check if already installed
    if proot-distro list | grep -q "debian"; then
        log_info "Debian already installed"
    else
        proot-distro install debian
    fi
    
    # Configure login alias
    echo "alias debian='proot-distro login debian'" >> ~/.bashrc
    
    log_info "Proot Debian installed! Use 'debian' to enter"
}

setup_sudo_debian() {
    if ! check_proot; then
        log_warn "Not in proot debian. Run 'debian' first, then:"
        echo "  apt update && apt install -y sudo"
        return
    fi
    
    log_step "Setting up sudo in Debian..."
    
    apt update
    apt install -y sudo
    
    echo "Defaults !requiretty" | tee -a /etc/sudoers.d/norequiretty
    chmod 0440 /etc/sudoers.d/norequiretty
    
    log_info "Sudo configured!"
}

download_binary() {
    log_step "Downloading latest binary..."
    
    # Get latest release version
    VERSION=$(curl -s https://api.github.com/repos/JonyBepary/son-of-anthon/releases/latest | grep -o '"tag_name": "v[^"]*' | cut -d'"' -f4)
    
    if [ -z "$VERSION" ]; then
        log_error "Failed to get latest version"
        exit 1
    fi
    
    log_info "Latest version: $VERSION"
    
    # Download
    mkdir -p ~/son-of-anthon
    cd ~/son-of-anthon
    
    URL="https://github.com/JonyBepary/son-of-anthon/releases/download/${VERSION}/son-of-anthon_${VERSION#v}_android_arm64.tar"
    
    log_info "Downloading from: $URL"
    curl -L -o son-of-anthon.tar "$URL"
    
    # Extract
    tar -xf son-of-anthon.tar
    
    # Cleanup
    rm -f son-of-anthon.tar
    
    cd -
    
    log_info "Binary downloaded to ~/son-of-anthon/"
}

run_setup() {
    log_step "Running setup wizard..."
    
    if [ -f "$PREFIX/bin/$APP_NAME" ]; then
        $APP_NAME setup
    else
        log_warn "Binary not installed. Run install first."
    fi
}

install_service() {
    log_step "Installing runit service..."
    
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
    
    log_info "runit service created!"
}

enable_autostart() {
    log_step "Enabling autostart on Termux launch..."
    
    # Method 1: Using termux-boot (recommended for device boot)
    if ! command -v termux-boot &> /dev/null; then
        log_info "Installing termux-boot for device autostart..."
        pkg install -y termux-boot
    fi
    
    # Create autostart script
    BOOT_DIR="$PREFIX/var/lib/termux-boot"
    mkdir -p "$BOOT_DIR"
    
    cat > "$BOOT_DIR/start-sonofanthon.sh" << 'EOF'
#!/data/data/com.termux/files/usr/bin/sh
export HOME="/data/data/com.termux/files/home"
sv up son-of-anthon 2>/dev/null || true
EOF
    
    chmod +x "$BOOT_DIR/start-sonofanthon.sh"
    
    # Method 2: Add to .bashrc for when termux opens
    if ! grep -q "son-of-anthon" ~/.bashrc 2>/dev/null; then
        echo "" >> ~/.bashrc
        echo "# Auto-start son-of-anthon" >> ~/.bashrc
        echo "sv up son-of-anthon 2>/dev/null || true" >> ~/.bashrc
    fi
    
    log_info "Autostart configured!"
}

start_service() {
    log_step "Starting service..."
    
    sv up $SERVICE_NAME
    
    sleep 2
    
    if sv status $SERVICE_NAME | grep -q "run"; then
        log_info "Service started successfully!"
    else
        log_warn "Service may not have started. Check: sv status $SERVICE_NAME"
    fi
}

show_status() {
    echo ""
    echo "============================================"
    echo "  Son of Anthon - Status"
    echo "============================================"
    echo ""
    
    echo -n "Service: "
    if sv status $SERVICE_NAME 2>/dev/null | grep -q "run"; then
        echo -e "${GREEN}Running${NC}"
    else
        echo -e "${RED}Stopped${NC}"
    fi
    
    echo -n "Binary: "
    if [ -f "$PREFIX/bin/$APP_NAME" ]; then
        echo -e "${GREEN}Installed${NC}"
    else
        echo -e "${RED}Not installed${NC}"
    fi
    
    echo -n "Config: "
    if [ -f "$CONFIG_DIR/config.json" ]; then
        echo -e "${GREEN}Configured${NC}"
    else
        echo -e "${YELLOW}Run setup${NC}"
    fi
    
    echo ""
    echo "Commands:"
    echo "  son-of-anthon onboard --full    # Full setup"
    echo "  son-of-anthon onboard --start  # Just start service"
    echo "  sv status $SERVICE_NAME        # Check status"
    echo "  sv down $SERVICE_NAME          # Stop"
    echo "  tail -f $CONFIG_DIR/termux-logs/current  # View logs"
    echo ""
}

main() {
    case "$1" in
        --full|--all)
            install_deps
            if [ "$2" = "--debian" ] || [ "$2" = "--deb" ]; then
                install_proot_debian
            fi
            download_binary
            $PWD/son-of-anthon/install/termux/install.sh
            enable_autostart
            start_service
            show_status
            ;;
        --debian|--deb)
            install_proot_debian
            ;;
        --sudo)
            setup_sudo_debian
            ;;
        --deps)
            install_deps "$2"
            ;;
        --download)
            download_binary
            ;;
        --install)
            $PWD/son-of-anthon/install/termux/install.sh
            ;;
        --autostart)
            enable_autostart
            ;;
        --start)
            start_service
            ;;
        --status)
            show_status
            ;;
        *)
            echo "Son of Anthon - Onboard Command"
            echo ""
            echo "Usage: son-of-anthon onboard [option]"
            echo ""
            echo "Options:"
            echo "  --full           Full setup (deps + download + install + autostart)"
            echo "  --full --debian Full setup + install Proot Debian"
            echo "  --debian        Install Proot Debian"
            echo "  --sudo          Configure sudo in Debian (run inside debian)"
            echo "  --deps          Install base dependencies"
            echo "  --download      Download latest binary"
            echo "  --install       Run installer"
            echo "  --autostart     Enable autostart on Termux launch"
            echo "  --start         Start the service"
            echo "  --status        Show status"
            echo ""
            echo "Quick start:"
            echo "  son-of-anthon onboard --full"
            echo ""
            ;;
    esac
}

main "$@"
