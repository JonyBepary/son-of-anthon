package chief

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jony/son-of-anthon/pkg/skills"
	"github.com/sipeed/picoclaw/pkg/tools"
)

type ChiefSkill struct {
	workspace string
}

func NewSkill() *ChiefSkill {
	return &ChiefSkill{}
}

func (s *ChiefSkill) Name() string {
	return "chief"
}

func (s *ChiefSkill) Description() string {
	return `Chief of Staff - Strategic orchestrator who aggregates all agent outputs into briefings.

Commands:
- morning_brief: Compile today's tasks (ATC), news (Monitor), research (Research), deadlines (Architect) into a single morning brief and save it.
- evening_review: Compile completed tasks (ATC), learning (Coach), productivity stats, and tomorrow's prep into an evening review.
- urgent_deadlines: Check deadlines-today.md for items due within 2 hours and return alert or silent OK.
- delegate: Route a task to the appropriate specialist agent (returns guidance for subagent tool).
- status: Show which agent workspaces are active.`
}

func (s *ChiefSkill) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command to execute",
				"enum":        []string{"morning_brief", "evening_review", "urgent_deadlines", "delegate", "status"},
			},
			"task": map[string]interface{}{
				"type":        "string",
				"description": "Task to delegate (for delegate command)",
			},
			"agent": map[string]interface{}{
				"type":        "string",
				"description": "Target agent (for delegate command)",
				"enum":        []string{"architect", "atc", "coach", "monitor", "research"},
			},
		},
		"required": []string{"command"},
	}
}

func (s *ChiefSkill) SetWorkspace(ws string) {
	s.workspace = ws
	s.initWorkspace()
}

func (s *ChiefSkill) initWorkspace() {
	if s.workspace == "" {
		return
	}
	os.MkdirAll(s.workspace, 0755)

	identityPath := filepath.Join(s.workspace, "IDENTITY.md")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		identityContent := `# Chief of Staff - Identity

- **Name:** Chief
- **Creature:** Strategic commander with clipboard
- **Vibe:** "I've got the big picture"
- **Emoji:** 🎯
- **Catchphrase:** "Here's the plan..."

## Role
Chief of Staff who coordinates specialist agents, synthesizes their outputs, and delivers morning briefs + evening reviews.

**I'm Chief. Let's get things done.** 🎯
`
		os.WriteFile(identityPath, []byte(identityContent), 0644)
	}

	heartbeatPath := filepath.Join(s.workspace, "HEARTBEAT.md")
	if _, err := os.Stat(heartbeatPath); os.IsNotExist(err) {
		heartbeatContent := `# HEARTBEAT.md

# Keep this file empty (or with only comments) to skip heartbeat API calls.

# Add tasks below when you want the agent to check something periodically.
`
		os.WriteFile(heartbeatPath, []byte(heartbeatContent), 0644)
	}

	memDir := filepath.Join(s.workspace, "memory")
	os.MkdirAll(memDir, 0755)
}

func (s *ChiefSkill) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	command, _ := args["command"].(string)

	switch command {
	case "morning_brief":
		return s.executeMorningBrief(ctx, args)
	case "evening_review":
		return s.executeEveningReview(ctx, args)
	case "urgent_deadlines":
		return s.executeUrgentDeadlines(ctx, args)
	case "delegate":
		return s.executeDelegate(ctx, args)
	case "status":
		return s.executeStatus(ctx, args)
	default:
		return tools.ErrorResult(fmt.Sprintf("Unknown command: %s", command))
	}
}

// ----------------------------------------------------------------------------
// MORNING BRIEF
// ----------------------------------------------------------------------------

func (s *ChiefSkill) executeMorningBrief(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	now := time.Now()
	var brief strings.Builder

	brief.WriteString(fmt.Sprintf("# 🎯 Morning Brief — %s\n\n", now.Format("Monday, January 2, 2006")))

	brief.WriteString("## ✈️ Today's Tasks (ATC)\n")
	brief.WriteString(s.getTodaysFocus())
	brief.WriteString("\n\n")

	brief.WriteString("## 📋 Urgent Deadlines (Architect)\n")
	brief.WriteString(s.getDeadlinesFile())
	brief.WriteString("\n\n")

	brief.WriteString("## 🌍 News (Monitor)\n")
	brief.WriteString(s.getNewsHighlights(now))
	brief.WriteString("\n\n")

	brief.WriteString("## 🔬 Research (Research)\n")
	brief.WriteString(s.getResearchUpdates(now))
	brief.WriteString("\n\n")

	brief.WriteString("## 📚 Learning (Coach)\n")
	brief.WriteString(s.getLearningFile("learning-today.md"))
	brief.WriteString("\n\n")

	brief.WriteString("---\n**Ready to roll? 🚀**\n")

	output := brief.String()
	s.saveBrief(output, "morning-brief")

	return &tools.ToolResult{ForLLM: output, ForUser: output}
}

// getTodaysFocus parses ATC's tasks.xml and returns urgency-scored Today tasks.
func (s *ChiefSkill) getTodaysFocus() string {
	tasksPath := filepath.Join(s.workspace, "..", "atc", "memory", "tasks.xml")
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		return "- ⚠️ ATC tasks.xml not found. Run `atc analyze_tasks` first.\n"
	}

	// Minimal inline xCal parse — just what Chief needs
	type prop struct {
		Text string `xml:",chardata"`
	}
	type vtodoProp struct {
		Summary    prop `xml:"summary>text"`
		Status     prop `xml:"status>text"`
		Categories prop `xml:"categories>text"`
	}
	type vtodo struct {
		Properties vtodoProp `xml:"properties"`
	}
	type components struct {
		VTodos []vtodo `xml:"vtodo"`
	}
	type vcal struct {
		Components components `xml:"components"`
	}
	type ical struct {
		VCal vcal `xml:"vcalendar"`
	}

	var cal ical
	if err := xml.Unmarshal(data, &cal); err != nil {
		return fmt.Sprintf("- ⚠️ Failed to parse tasks.xml: %v\n", err)
	}

	var sb strings.Builder
	count := 0
	for _, todo := range cal.VCal.Components.VTodos {
		cat := strings.ToLower(todo.Properties.Categories.Text)
		status := strings.ToLower(todo.Properties.Status.Text)
		if strings.Contains(cat, "today") && status != "completed" {
			sb.WriteString(fmt.Sprintf("- %s\n", todo.Properties.Summary.Text))
			count++
		}
	}
	if count == 0 {
		return "- No active tasks for today in tasks.xml.\n"
	}
	return sb.String()
}

// getDeadlinesFile reads the Architect-written deadlines file.
func (s *ChiefSkill) getDeadlinesFile() string {
	return s.readMemoryFile("deadlines-today.md", "- No deadlines file found. Architect hasn't written one yet.\n")
}

// getLearningFile reads Coach-written learning files by name.
func (s *ChiefSkill) getLearningFile(name string) string {
	return s.readMemoryFile(name, "- No learning data (Coach not yet configured).\n")
}

// getNewsHighlights reads Monitor's RFC news cache for today / yesterday.
// Caps at K=20 entries. Passively GCs expired files (TTL=6h).
func (s *ChiefSkill) getNewsHighlights(now time.Time) string {
	for _, d := range []string{now.Format("20060102"), now.AddDate(0, 0, -1).Format("20060102")} {
		path := filepath.Join(s.workspace, "memory", "news-"+d+".md")
		lines, err := skills.ParseRFCFile(path, 20)
		if err == nil && len(lines) > 0 {
			return strings.Join(lines, "\n") + "\n"
		}
	}
	return "- No news cache found. Run 'fetch news' to populate.\n"
}

// getResearchUpdates reads Research's RFC paper cache for today / yesterday.
// Caps at K=15 entries. Passively GCs expired files (TTL=24h).
func (s *ChiefSkill) getResearchUpdates(now time.Time) string {
	for _, d := range []string{now.Format("20060102"), now.AddDate(0, 0, -1).Format("20060102")} {
		path := filepath.Join(s.workspace, "memory", "research-"+d+".md")
		lines, err := skills.ParseRFCFile(path, 15)
		if err == nil && len(lines) > 0 {
			return strings.Join(lines, "\n") + "\n"
		}
	}
	return "- No research cache found. Run 'search papers' to populate.\n"
}

// ----------------------------------------------------------------------------
// EVENING REVIEW
// ----------------------------------------------------------------------------

func (s *ChiefSkill) executeEveningReview(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	now := time.Now()
	var review strings.Builder

	review.WriteString(fmt.Sprintf("# 🌙 Evening Review — %s\n\n", now.Format("Monday, January 2, 2006")))

	review.WriteString("## ✅ Completed Tasks (ATC)\n")
	review.WriteString(s.getCompletedTasks())
	review.WriteString("\n\n")

	review.WriteString("## 📚 Learning (Coach)\n")
	review.WriteString(s.getLearningFile("learning-today.md"))
	review.WriteString("\n\n")

	review.WriteString("## 📊 Productivity Stats (ATC)\n")
	review.WriteString(s.readMemoryFile("stats-today.md", "- No stats yet. ATC will write during evening roll-over.\n"))
	review.WriteString("\n\n")

	review.WriteString("## 🔬 Tomorrow's Research\n")
	review.WriteString(s.readMemoryFile("tomorrow/research.md", "- Not pre-fetched yet.\n"))
	review.WriteString("\n\n")

	review.WriteString("## 🌍 Tomorrow's News\n")
	review.WriteString(s.readMemoryFile("tomorrow/news.md", "- Not pre-fetched yet.\n"))
	review.WriteString("\n\n")

	review.WriteString("---\n**Good work today. Rest well. 🌙**\n")

	output := review.String()
	s.saveBrief(output, "evening-review")

	return &tools.ToolResult{ForLLM: output, ForUser: output}
}

// getCompletedTasks parses ATC tasks.xml for COMPLETED items.
func (s *ChiefSkill) getCompletedTasks() string {
	tasksPath := filepath.Join(s.workspace, "..", "atc", "memory", "tasks.xml")
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		return "- ⚠️ ATC tasks.xml not found.\n"
	}

	type prop struct {
		Text string `xml:",chardata"`
	}
	type vtodoProp struct {
		Summary prop `xml:"summary>text"`
		Status  prop `xml:"status>text"`
	}
	type vtodo struct {
		Properties vtodoProp `xml:"properties"`
	}
	type components struct {
		VTodos []vtodo `xml:"vtodo"`
	}
	type vcal struct {
		Components components `xml:"components"`
	}
	type ical struct {
		VCal vcal `xml:"vcalendar"`
	}

	var cal ical
	if err := xml.Unmarshal(data, &cal); err != nil {
		return fmt.Sprintf("- ⚠️ Failed to parse tasks.xml: %v\n", err)
	}

	var sb strings.Builder
	count := 0
	for _, todo := range cal.VCal.Components.VTodos {
		if strings.EqualFold(todo.Properties.Status.Text, "completed") {
			sb.WriteString(fmt.Sprintf("- ✅ %s\n", todo.Properties.Summary.Text))
			count++
		}
	}
	if count == 0 {
		return "- No completed tasks yet today.\n"
	}
	return sb.String()
}

// ----------------------------------------------------------------------------
// URGENT DEADLINES (Heartbeat workflow)
// ----------------------------------------------------------------------------

func (s *ChiefSkill) executeUrgentDeadlines(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	content := s.readMemoryFile("deadlines-today.md", "")
	if content == "" {
		msg := "✅ No deadlines file found. Silent OK."
		return &tools.ToolResult{ForLLM: msg, ForUser: msg}
	}

	now := time.Now()
	var urgent []string

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Look for ISO timestamps in the line, e.g. 2026-02-20T17:00:00
		if idx := strings.Index(line, "20"); idx >= 0 {
			sub := line[idx:]
			if len(sub) >= 16 {
				candidate := sub[:16] // "2026-02-20T17:00"
				if t, err := time.ParseInLocation("2006-01-02T15:04", candidate, now.Location()); err == nil {
					hoursLeft := t.Sub(now).Hours()
					if hoursLeft >= 0 && hoursLeft < 2 {
						urgent = append(urgent, fmt.Sprintf("  • %s — due in %.0f min", line, t.Sub(now).Minutes()))
					}
				}
			}
		}
	}

	if len(urgent) == 0 {
		msg := "✅ No urgent deadlines (all ≥ 2h away). Silent OK."
		return &tools.ToolResult{ForLLM: msg, ForUser: msg}
	}

	alert := "⚠️ URGENT DEADLINES:\n" + strings.Join(urgent, "\n") + "\n\nTime to focus! 🎯"
	return &tools.ToolResult{ForLLM: alert, ForUser: alert}
}

// ----------------------------------------------------------------------------
// DELEGATE
// ----------------------------------------------------------------------------

func (s *ChiefSkill) executeDelegate(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	task, _ := args["task"].(string)
	agent, _ := args["agent"].(string)

	if task == "" {
		return tools.ErrorResult("task is required for delegate command")
	}
	if agent == "" {
		agent = s.detectAgent(task)
	}

	result := fmt.Sprintf("**Delegating to %s:** %s\n\nUse the subagent tool to spawn `%s` with this task message.", agent, task, agent)
	return &tools.ToolResult{ForLLM: result, ForUser: result}
}

func (s *ChiefSkill) detectAgent(task string) string {
	taskLower := strings.ToLower(task)
	if strings.Contains(taskLower, "paper") || strings.Contains(taskLower, "research") || strings.Contains(taskLower, "arxiv") {
		return "research"
	}
	if strings.Contains(taskLower, "news") || strings.Contains(taskLower, "monitor") || strings.Contains(taskLower, "bangladesh") {
		return "monitor"
	}
	if strings.Contains(taskLower, "task") || strings.Contains(taskLower, "priority") || strings.Contains(taskLower, "calendar") {
		return "atc"
	}
	if strings.Contains(taskLower, "learn") || strings.Contains(taskLower, "ielts") || strings.Contains(taskLower, "study") {
		return "coach"
	}
	if strings.Contains(taskLower, "bill") || strings.Contains(taskLower, "deadline") || strings.Contains(taskLower, "medicine") {
		return "architect"
	}
	return "atc"
}

// ----------------------------------------------------------------------------
// STATUS
// ----------------------------------------------------------------------------

func (s *ChiefSkill) executeStatus(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	var sb strings.Builder
	sb.WriteString("# 🎯 System Status\n\n")

	agents := []string{"architect", "atc", "chief", "coach", "monitor", "research"}
	for _, agent := range agents {
		agentPath := filepath.Join("workspaces", agent)
		if _, err := os.Stat(agentPath); err == nil {
			// Check if memory dir has recent files
			memPath := filepath.Join(agentPath, "memory")
			entries, _ := os.ReadDir(memPath)
			sb.WriteString(fmt.Sprintf("- ✅ **%s**: active (%d memory files)\n", agent, len(entries)))
		} else {
			sb.WriteString(fmt.Sprintf("- ⏳ **%s**: workspace not found\n", agent))
		}
	}

	out := sb.String()
	return &tools.ToolResult{ForLLM: out, ForUser: out}
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// readMemoryFile reads a file from the chief workspace memory dir.
// Returns fallback string if file is missing or empty.
func (s *ChiefSkill) readMemoryFile(name, fallback string) string {
	if s.workspace == "" {
		return fallback
	}
	path := filepath.Join(s.workspace, "memory", name)
	data, err := os.ReadFile(path)
	if err != nil {
		// If it's deadlines-today or learning-today, some old caching might use architect/coach, but let's strictly look in chief/memory here or its own subagent.
		// Wait, user designed the other agents to write to their respective memory. If deadlines-today is in architect, we must check there.
		path = filepath.Join(s.workspace, "..", "architect", "memory", name)
		data, err = os.ReadFile(path)
		if err != nil {
			path = filepath.Join(s.workspace, "..", "coach", "memory", name)
			data, err = os.ReadFile(path)
			if err != nil {
				return fallback
			}
		}
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return fallback
	}
	return content + "\n"
}

// saveBrief writes the brief to chief/memory/TYPE-YYYY-MM-DD.md.
func (s *ChiefSkill) saveBrief(content, briefType string) {
	if s.workspace == "" {
		return
	}
	memoryDir := filepath.Join(s.workspace, "memory")
	os.MkdirAll(memoryDir, 0755)
	filename := fmt.Sprintf("%s-%s.md", briefType, time.Now().Format("2006-01-02"))
	path := filepath.Join(memoryDir, filename)
	os.WriteFile(path, []byte(content), 0644)
}
