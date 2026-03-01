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

Download from [Releases](https://github.com/JonyBepary/son-of-anthon/releases) for your platform:

```bash
# Linux
sudo apt install ./son-of-anthon_*.deb    # Debian/Ubuntu
sudo dnf install ./son-of-anthon_*.rpm    # Fedora/RHEL

# Termux
apt install ./son-of-anthon_*.deb

# Or clone and build
git clone https://github.com/JonyBepary/son-of-anthon.git
cd son-of-anthon
git submodule update --init --recursive
make build-all

# Run setup
./son-of-anthon gateway
```

---

## Installation

### Option 1: Native Package (Recommended)

| Platform | Package | Command |
|----------|---------|---------|
| Ubuntu/Debian | `.deb` | `sudo apt install ./son-of-anthon_*.deb` |
| Fedora/RHEL | `.rpm` | `sudo dnf install ./son-of-anthon_*.rpm` |
| Termux | `.deb` | `apt install ./son-of-anthon_*.deb` |

Native packages automatically:
- Install binary to `/usr/bin`
- Register service (systemd/runit)
- Enable on boot

### Option 2: Manual Install

#### Linux/macOS/Termux
```bash
# Download from Releases: https://github.com/JonyBepary/son-of-anthon/releases
tar -xzf son-of-anthon_*.tar
cd son-of-anthon_*
./install.sh
```

#### Windows
```bash
# Download son-of-anthon_*.zip from Releases
# Extract and run as Administrator:
install.bat
```

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
