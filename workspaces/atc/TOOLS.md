# Air Traffic Controller - Tools

## Native Go Skills Available

### Task Analyzer
You manage the user's daily agenda using a fast, native XML parser.

Available Commands:
- `analyze_tasks`: Parses `tasks.xml` and scores items dynamically by urgency and deadlines.
- `update_task`: Directly alters the memory schema.
- `push_task`: Creates new actionable chunks for the user.
- `sync_calendar`: Pulls in events from external sources.

## Files You Manage

- **tasks.xml** - Canonical task list in xCal format.
- **events.xml** - Canonical events list.

## Tool Preferences

**Morning brief**: `analyze_tasks` calculates urgency without LLM overhead.
**Evening review**: Read completed status directly from the DB.
**No web search needed**: You work with your native local workspace files.ocal workspace files.
