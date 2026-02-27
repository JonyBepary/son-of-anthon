# Agent Profiles

Son of Anthon contains a unified team of seven specialized agents. The Chief orchestrates them when you ask for morning briefs or evening reviews. You can also interact with them directly via the `subagent` command.

## 1. Chief
**The Strategic Commander**
The Chief is the master orchestration agent. When provided triggers like "Generate morning brief", it executes its fully native Go workflows to pull inputs from Architect, ATC, Research, and Monitor, synthesize the entire context, and output an elegantly formatted summary for the user to review. It also runs Zero-Cost checks against urgent deadlines via the daemon's heartbeat.

## 2. Air Traffic Controller (ATC)
**The Agenda Manager**
ATC parses and scores the user's agenda dynamically without relying on massive LLM prompts. By natively indexing `tasks.xml` and `events.xml` in valid xCal formats, ATC is capable of returning `[P0]`, `[P1]` prioritized task chunks directly to the Chief's morning brief and tracking what is completed by the evening review.

## 3. Architect
**The Life Logistics Planner**
Architect natively hooks into Nextcloud CalDAV solutions to `sync_deadlines`, `create_task`, and `delete_task`. It pulls upcoming recurring life admin tasks (like medication orders, rent, etc.) securely and tracks their cyclical completion. 

## 4. Coach
**Habit & Streak Tracker**
Coach encourages the user and tracks learning streaks natively within an SQLite database. It calculates productive metrics independently and supplies the Evening Review with an aggregated understanding of the user's focus time.

## 5. Monitor
**The News Curator**
Monitor completely circumvents legacy Python SearXNG scrapers. It reads from an expansive, 150+ source OPML file (`feeds.opml`) embedded locally. Using pure Go, it fetches breaking headlines, deduplicates them natively, and provides high-signal intelligence without any proprietary tracking algorithms.

## 6. Research
**Academic Explorer**
Research hooks into academic databases (like ArXiv) natively. When asked to prep for tomorrow, it pulls the most recently published papers matching the user's specific context keywords, providing abstracts directly into memory.

## Subagent Orchestration
**Contextual Spawner Layer**
The `subagent` is not a separate personality, but rather the underlying orchestration tool that allows Chief or any other agent to easily spin up deeply scoped variations of the bots above for long-running or isolated tasks. Instead of dumping heavy research into the main chat window, it delegates the heavy lifting to an invisible cloned thread.
