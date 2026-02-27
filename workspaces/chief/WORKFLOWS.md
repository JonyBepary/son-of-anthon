# Chief's Orchestration Workflows

**CRITICAL INSTRUCTIONS:** Chief has powerful native Go methods to orchestrate the daily operations without manually spawning hundreds of subagents.

---

## Morning Brief Orchestration

**Trigger:** Message contains "Generate morning brief" (case-insensitive)

**EXECUTION STEPS:**
1. Call the `morning_brief` tool directly.
2. Synthesize the output and present it elegantly to the user.

---

## Evening Review Orchestration

**Trigger:** Message contains "Generate evening review" (case-insensitive)

**Missing Output:**
- File doesn't exist after agent completes
- Include in brief: "⚠️ [Agent] completed but no data available"

**Partial Brief:**
- Always send brief with whatever data is available
- Adapt Strategic Call to acknowledge missing data
- Provide fallback guidance

---

## File Validation

Before reading any output file:
1. **Exists:** FileNotFoundError → skip, use error message
2. **Not Empty:** Size = 0 → skip, use error message
3. **Fresh:** Modified >10min ago → warn but use anyway

---

## Expected Output Files

- Architect → memory/deadlines-today.md
- Monitor → memory/news-YYYY-MM-DD.md
- ATC → memory/tasks-today.md (morning) or memory/stats-today.md (evening)
- Research → memory/research-YYYY-MM-DD.md (morning) or memory/tomorrow/research.md (evening)
- Coach → memory/learning-today.md
