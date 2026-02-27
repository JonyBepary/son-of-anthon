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
- 📰 **Monitor** - RSS news curation
- 🔬 **Research** - Academic paper discovery

## Quick Start

### 1. Clone & Build

```bash
git clone https://github.com/JonyBepary/son-of-anthon.git
cd son-of-anthon

# Initialize submodule
git submodule update --init --recursive

# Build
go build -o son-of-anthon ./cmd/son-of-anthon
```

### 2. Run Setup

```bash
./son-of-anthon setup
```

This will:
- Create `~/.picoclaw/` directory
- Extract workspace templates
- Prompt for API keys (NVIDIA, Telegram, Nextcloud, Brave)

### 3. Start Gateway

```bash
./son-of-anthon gateway
```

## Termux Installation (Android)

See [setup-termux.sh](./setup-termux.sh) for full instructions.

```bash
# Install dependencies
pkg install termux-services git golang

# Clone and build
git clone https://github.com/JonyBepary/son-of-anthon.git
cd son-of-anthon
git submodule update --init --recursive
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o son-of-anthon-termux ./cmd/son-of-anthon

# Run setup
./son-of-anthon-termux setup

# Setup runit service
bash setup-termux.sh

# Start daemon
sv up son-of-anthon
```

## Configuration

Copy `config.example.json` to `~/.picoclaw/config.json` and fill in your keys:

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

## Requirements

- Go 1.26+
- Telegram Bot Token (optional)
- NVIDIA API key (for LLM)
- Nextcloud instance (optional, for calendar/tasks)

## Status

🚧 **UNDER CONSTRUCTION** - Not ready for production use

- API may change
- Features incomplete
- Testing in progress

## License

MIT
