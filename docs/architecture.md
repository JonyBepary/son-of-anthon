# Architecture & Mechanics

Son of Anthon is designed around a central Gateway daemon that binds user interfaces (Telegram, CLI) to an underlying graph of Autonomous Agents provided by `picoclaw`.

## The Gateway (`cmd/son-of-anthon/main.go`)

The gateway is the entry point for the application. When executed as `./son-of-anthon gateway`, it performs the following:

1. **Bootstrapping**: Checks if `~/.picoclaw/config.json` and `~/.picoclaw/workspace/` exist. If missing, it uses `go:embed` to securely extract default settings and markdown templates natively from the binary without touching shell commands.
2. **Provider Initialization**: Loads the LLM provider (e.g., OpenRouter, OpenAI, Ollama) and attaches it to the `picoclaw` agent core.
3. **Skill Registration**: Binds all native Go skills located in `pkg/skills/` to the primary agent engine.
4. **Daemon Loops**: 
   - Starts the **Telegram Channel Manager** in polling mode.
   - Starts the **Cron Manager** for scheduled autonomous jobs (morning brief, evening review).
   - Starts the **Heartbeat Service** which wakes the system up on interval to check for urgent deadlines (Zero-Cost checks).

## The PicoClaw Core

We use a heavily customized integration with `github.com/sipeed/picoclaw`:

- **Agents**: The core LLM loop that executes multi-step logic.
- **Bus**: The asynchronous event horizon. It routes commands, logs, and outputs between agents.
- **Channels**: The I/O layer. We primarily use the `telegram` channel to receive text messages and dispatch `message(to="telegram")` payloads.
- **Heartbeat**: A specialized low-overhead ticker that triggers internal reviews without waiting for user input.

## Go-Native Skills

Previous iterations relied heavily on external Python and bash scripts. We have refactored all heavy lifting into native Go functions under `pkg/skills/`.

- **No Sub-processes**: We no longer shell out to Python.
- **Native Parsers**: XML (`atc`), SQLite (`coach`), and direct HTTP requests (`monitor`, `research`) are all executed in pure Go natively alongside the LLM.
- **Dynamic Tool Defs**: Every skill must implement a `Parameters()` method returning a JSON Schema of its tool definition. This is automatically parsed and injected into the system prompt for the overarching LLM.
