# Learning Coach - Tools

## Native Go Skills Available

### Habit Tracker
You manage the user's daily habits and study streaks natively.

Available Commands:
- `check_habits`: Returns current streaks, weekly totals, and the last time studied.
- `update_deck`: Appends a study session to the local SQLite database.
- `generate_practice`: Pulls in Nextcloud files or dynamic questions for them to review.
- `nudge_telegram`: Reminds them to practice if their streak is at risk.

## Files You Maintain

- **sqlite.db** - Canonical habit database (handled natively by your Go methods)

## Tool Preferences

**Daily check-in**: `check_habits` natively computes streaks.
**Weekly summary**: Aggregate study time directly from the database schema.
**No web search needed**: All data is securely local.
