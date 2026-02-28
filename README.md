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

### 1. Clone & Build

```bash
git clone https://github.com/JonyBepary/son-of-anthon.git
cd son-of-anthon
git submodule update --init --recursive
go build -o son-of-anthon ./cmd/son-of-anthon
```

### 2. Run Setup

```bash
./son-of-anthon setup
```

This will prompt for API keys (NVIDIA, Telegram, Nextcloud).

### 3. Start Gateway

```bash
./son-of-anthon gateway
```

---

## Installation Guides

### Termux (Android)

```bash
# Copy son-of-anthon-termux to phone
cp son-of-anthon-termux ~/storage/downloads/

# In Termux:
cd ~/storage/downloads
bash setup-termux.sh son-of-anthon-termux

# Start daemon
sv up son-of-anthon

# Check logs
tail -f ~/.picoclaw/termux-logs/current
```

### Linux (systemd)

```bash
# As root:
sudo ./install/install-systemd.sh

# Enable on boot
sudo systemctl enable son-of-anthon

# Start now
sudo systemctl start son-of-anthon

# View logs
journalctl -u son-of-anthon -f
```

### Windows

1. Download `son-of-anthon-windows-amd64.exe` from releases
2. Run `install/setup-windows.bat` as Administrator
3. Edit `%APPDATA%\son-of-anthon\config.json` with your API keys
4. Run `son-of-anthon.exe gateway`

---

## Configuration

Copy `config.example.json` to `~/.picoclaw/config.json`:

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

Default feeds configured in `config.json`:
- **Bangladesh**: Prothom Alo, The Daily Star, bdnews24
- **World**: Google News
- **AI**: OpenAI, GPT, Gemini, Claude
- **Tech**: Silicon Valley, Apple, Google, Microsoft
- **Finance**: Stock, crypto
- **Policy**: Government, election

---

## Requirements

- Go 1.26+
- Telegram Bot Token (optional)
- NVIDIA API key (for LLM)
- Nextcloud (optional)

---

## Status

🚧 **UNDER CONSTRUCTION** - Not production ready

---

## License

MIT
