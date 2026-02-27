# Configuration Guide

The main configuration file is automatically created at `~/.picoclaw/config.json` upon the first execution of `./son-of-anthon gateway`. Modifying the root project `./config.json` after the first run will not affect the active environment.

## Example `config.json`

```json
{
  "provider": "openrouter",
  "api_key": "YOUR_OPENROUTER_KEY",
  "model": "qwen/qwen3.5-397b-a17b",
  },
  "tools": {
    "nextcloud": {
      "host": "https://ivo.lv.tab.digital",
      "username": "email@example.com",
      "password": "YOUR_APP_PASSWORD"
    },
    "telegram": {
      "bot_token": "YOUR_TELEGRAM_BOT_TOKEN",
      "chat_id": "YOUR_TELEGRAM_CHAT_ID"
    }
  },
  "heartbeat": {
    "enabled": true,
    "interval_minutes": 5
  }
}
```

## Key Parameters

1. **Provider**: Defines the LLM endpoint. We natively support `openrouter` out of the box to easily hot-swap massive open-source models like `qwen`.
2. **API Key**: Ensure this is securely kept.
3. **Tools - Telegram**:
   - `bot_token`: Create a bot via BotFather and inject the token here. The gateway daemon handles polling automatically.
   - `chat_id`: Pass your exact numerical Chat ID (e.g., `1559319830`) to restrict interaction.
4. **Tools - Nextcloud**:
   - `host`: Your Nextcloud instance URL (e.g., `https://ivo.lv.tab.digital`). The agent will dynamically construct all the necessary sub-routes for CalDAV (`/remote.php/dav/calendars/...`), WebDAV (`/remote.php/webdav/...`), and the Deck API (`/index.php/apps/deck/api/...`) from this base host.
   - **Advanced Config:** If you are using custom internal paths or separate services for Tasks/Files/Deck, you can replace `host` with four explicit URLs: `calendar_url`, `tasks_url`, `files_url`, and `deck_url`.
4. **Heartbeat**: 
    - The interval checks `urgent_deadlines` via the Chief. The `interval_minutes` specifies the frequency.

## Workspace Customization

Beyond `config.json`, the daemon auto-extracts agent templates into `~/.picoclaw/workspace/`. 

To edit an agent's prompts or persona:
1. Navigate to `~/.picoclaw/workspace/<agent_name>/`
2. Open `IDENTITY.md` or `TOOLS.md`
45: 3. Restart the `./son-of-anthon gateway` process.
46: 
47: ## Android Termux 24/7 Deployment
48: 
49: Son of Anthon can run continuously as a background daemon on Android via [Termux](https://termux.dev/) using `termux-services`. This allows the agent to handle Telegram messages, cron jobs, and deadlines synchronously without you needing to keep the terminal open.
50: 
51: ### 1. Setup the Daemon
52: 
53: We provide an automated setup script that installs the necessary dependencies and configures the `runit` service structure for both the gateway and its logger:
54: 
55: ```bash
56: cd ~/pico-son-of-anthon
57: chmod +x setup-termux.sh
58: ./setup-termux.sh
59: ```
60: 
61: ### 2. Manage the Service
62: 
63: The service is named `son-of-anthon`. You can control it using the standard `sv` commands natively in Termux:
64: 
65: - **Start the daemon (runs in background across reboots):**
66:   ```bash
67:   sv up son-of-anthon
68:   ```
69: - **Check the status:**
70:   ```bash
71:   sv status son-of-anthon
72:   ```
73: - **Stop the daemon:**
74:   ```bash
75:   sv down son-of-anthon
76:   ```
77: 
78: ### 3. View Live Logs
79: 
80: Output and errors from the daemon are captured automatically via `svlogd` and piped to a dedicated log directory. To watch the agents' internal monologue and operations live:
81: 
82: ```bash
83: tail -f ~/pico-son-of-anthon/termux-logs/current
84: ```
