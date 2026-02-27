# Son of Anthon 🎯

**Son of Anthon** is a lightweight, Go-native multi-agent AI assistant orchestrator built on top of the [PicoClaw](https://github.com/sipeed/picoclaw) framework. It acts as a personal OS, orchestrating a team of specialized subagents to perform research, manage calendars, track habits, monitor global news feeds, and operate autonomously in the background via a daemonized gateway.

## Features

- **Multi-Agent Architecture**: Six specialized agents (`chief`, `architect`, `atc`, `coach`, `monitor`, `research`) that talk to each other and share context.
- **Subagent Orchestration**: The main AI can dynamically spawn isolated, cloned "subagents" in the background to tackle deep, complex research tasks without muddying the main chat context.
- **Go-Native Skills**: No more fragile python wrappers. Native XML parsing, SQLite integrations, and concurrent web crawlers directly in Go.
- **Zero-Dependency Workspaces**: Uses `go:embed` to package agent prompts and tool definitions directly into the binary. Workspaces auto-initialize seamlessly on first run.
- **Background Daemon**: Fully asynchronous Telegram polling, cron scheduling, and "Zero-Cost Heartbeats" to monitor deadlines and ping the user only when necessary.

## Getting Started

### Installation

Clone the repository and build the binary:

```bash
git clone https://github.com/jony/son-of-anthon.git
cd son-of-anthon
make build
```

### Android (Termux) Deployment

Want to run the `gateway` persistently in the background on your Android phone using Termux? We've included a helper script that auto-configures `termux-services`:

```bash
# 1. Run the auto-setup script
chmod +x setup-termux.sh
./setup-termux.sh

# 2. Start the daemon in the background
sv up son-of-anthon

# 3. (Optional) Check live logs
tail -f ~/pico-son-of-anthon/termux-logs/current
```

### Configuration

The first time you run the gateway, it will auto-extract the default configuration and workspaces:

```bash
./son-of-anthon gateway
```

**1. Configuration File:**
Your configuration is extracted to `~/.picoclaw/config.json`. Update this file with your API keys (e.g., OpenRouter, Telegram bot token).

**2. Workspace Definitions:**
Your agent workspaces (`IDENTITY.md`, `HEARTBEAT.md`, databases, etc.) are extracted to `~/.picoclaw/workspace/`. You can edit these files dynamically to tweak the agents' personalities and tool preferences without recompiling.

### Usage

Run the agent daemon in the background to handle Telegram messages, cron jobs, and heartbeat deadline trackers:

```bash
./son-of-anthon gateway
```

Interact directly with the master system via Telegram by messaging your configured bot.

## Documentation

For deep dives into the system mechanics, see the `/docs` directory:

- [Architecture & Mechanics](docs/architecture.md): How the gateway, `picoclaw`, and skills interact.
- [Agent Roster](docs/agents.md): Descriptions of all subagents and their capabilities.
- [Configuration Guide](docs/configuration.md): Detailed explanation of `config.json` parameters.
