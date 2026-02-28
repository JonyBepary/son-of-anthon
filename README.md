# Son of Anthon ⚠️

> **WARNING: This project is under active development and is NOT production-ready.**
> 
> Use for testing/development only. APIs, features, and data formats may change.

A lightweight, Go-native multi-agent AI assistant orchestrator built on [PicoClaw](https://github.com/sipeed/picoclaw). Acts as a personal OS, running autonomously in the background as a daemon.

## Features

- 🤖 **Chief** - Master orchestration agent, morning briefs
- 📋 **ATC** - Task management with Nextcloud sync
- 🏠 **Architect** - Life admin, deadline tracking  
- 📚 **Coach** - Learning assistant, habit tracking
- 📰 **Monitor** - RSS news curation (Google News powered)
- 🔬 **Research** - Academic paper discovery

## Quick Start

### Download & Run

Download from [Releases](https://github.com/JonyBepary/son-of-anthon/releases) for your platform, or:

```bash
# Clone and build
git clone https://github.com/JonyBepary/son-of-anthon.git
cd son-of-anthon
git submodule update --init --recursive

# Build for your platform
make build-all

# Run setup
./son-of-anthon gateway
```

---

## Installation

### Termux (Android)

```bash
# Copy son-of-anthon-termux to phone Downloads
# Then in Termux:

cd ~/storage/downloads
bash son-of-anthon-termux/install.sh

# Or use the installer:
bash install.sh
```

### Linux (Ubuntu/Debian/Fedora/Arch)

```bash
# Download and extract release, then:
sudo ./install.sh

# Start service:
sudo systemctl start son-of-anthon
sudo systemctl enable son-of-anthon  # Enable on boot
```

### macOS

```bash
# Download and extract release, then:
chmod +x install.sh
sudo ./install.sh

# Start service:
launchctl start com.sonofanthon.gateway
```

### Windows

1. Download `son-of-anthon-windows-amd64.exe.zip` from releases
2. Extract to folder
3. Right-click `install.bat` → Run as Administrator

---

## Configuration

After first run, config is created at `~/.picoclaw/config.json`:

```json
{
  "agents": {
    "defaults": {
      "provider": "qwen",
      "model": "qwen/qwen3.5-397b-a17b"
    }
  },
  "model_list": [{
    "provider": "qwen",
    "api_key": "YOUR_NVIDIA_API_KEY",
    "api_base": "https://integrate.api.nvidia.com/v1"
  }],
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "YOUR_TELEGRAM_BOT_TOKEN"
    }
  }
}
```

## News Sources (Monitor)

Default feeds:
- **Bangladesh**: Prothom Alo, The Daily Star, bdnews24
- **World**: Google News
- **AI**: OpenAI, GPT, Gemini, Claude
- **Tech**: Apple, Google, Microsoft, NVIDIA
- **Finance**: Stock, crypto
- **Policy**: Government, election

---

## Requirements

- Go 1.26+ (for development)
- Telegram Bot Token (optional)
- NVIDIA API key (for LLM)
- Nextcloud (optional)

---

## Status

🚧 **UNDER CONSTRUCTION** - Not production ready

---

## License

MIT
