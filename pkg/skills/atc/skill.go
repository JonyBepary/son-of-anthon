package atc

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jony/son-of-anthon/pkg/skills/caldav"
	"github.com/sipeed/picoclaw/pkg/tools"
)

type ATCSkill struct {
	workspace string
}

func NewSkill() *ATCSkill {
	return &ATCSkill{}
}

func (s *ATCSkill) Name() string {
	return "atc"
}

func (s *ATCSkill) Description() string {
	return `Air Traffic Controller (ATC) - Task management and calendar integration for Atlas.

Local task commands (operate on tasks.xml and events.xml in workspace memory):
- analyze_tasks: Parse tasks.xml and return urgency-scored active tasks for today.
- read_calendar: Parse events.xml for today's events using local timezone.
- extract_keywords: Extract keywords from 'Tomorrow' tasks for pre-fetching.
- update_task: Change the status of a task in tasks.xml by UID (e.g. COMPLETED).
- roll_over_tasks: Move all pending 'Today' tasks to 'Tomorrow' in tasks.xml.

Nextcloud CalDAV commands (operate live on Nextcloud via network):
- sync_calendar: Fetch external .ics calendar from Nextcloud and overwrite events.xml.
- push_task: Create a new task in Nextcloud with summary, due, start, priority, notes.
- list_nextcloud_tasks: List all task hrefs in your Nextcloud tasks/ collection.
- get_task: Fetch a single task's full details from Nextcloud by href.
- merge_task: Update fields of an existing Nextcloud task by href.
- delete_task: Delete a specific Nextcloud task by href.`
}

func (s *ATCSkill) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command to execute",
				"enum":        []string{"analyze_tasks", "read_calendar", "extract_keywords", "update_task", "roll_over_tasks", "sync_calendar", "push_task", "list_nextcloud_tasks", "get_task", "merge_task", "delete_task"},
			},
			"task_uid": map[string]interface{}{
				"type":        "string",
				"description": "The UID of the task to update (only for update_task).",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"description": "The new status (e.g. COMPLETED, IN-PROCESS) (only for update_task).",
			},
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "Task title/summary (only for push_task).",
			},
			"due": map[string]interface{}{
				"type":        "string",
				"description": "Optional due date in RFC3339 format, e.g. 2026-02-21T17:00:00Z (only for push_task).",
			},
			"start": map[string]interface{}{
				"type":        "string",
				"description": "Optional start date in RFC3339 format (only for push_task).",
			},
			"priority": map[string]interface{}{
				"type":        "integer",
				"description": "Priority: 1=High, 5=Medium, 9=Low (only for push_task).",
			},
			"notes": map[string]interface{}{
				"type":        "string",
				"description": "Optional description/notes for the task (only for push_task).",
			},
			"task_href": map[string]interface{}{
				"type":        "string",
				"description": "The CalDAV href path of the task to delete, e.g. /remote.php/dav/calendars/user/tasks/uid.ics (only for delete_task).",
			},
		},
		"required": []string{"command"},
	}
}

func (s *ATCSkill) SetWorkspace(ws string) {
	s.workspace = ws
	s.initWorkspace()
}

func (s *ATCSkill) initWorkspace() {
	if s.workspace == "" {
		return
	}
	os.MkdirAll(s.workspace, 0755)

	identityPath := filepath.Join(s.workspace, "IDENTITY.md")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		identityContent := `# Air Traffic Controller - Identity

- **Name:** Atlas
- **Creature:** Calm air traffic controller with headset and coffee ☕
- **Vibe:** "I've got your back, here's what matters today"
- **Emoji:** ✈️
- **Catchphrase:** "Let's land this smoothly..."
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

	emptyXML := `<?xml version="1.0" encoding="utf-8"?>
<icalendar xmlns="urn:ietf:params:xml:ns:icalendar-2.0">
  <vcalendar>
    <components></components>
  </vcalendar>
</icalendar>`

	tasksPath := filepath.Join(memDir, "tasks.xml")
	if _, err := os.Stat(tasksPath); os.IsNotExist(err) {
		os.WriteFile(tasksPath, []byte(emptyXML), 0644)
	}

	eventsPath := filepath.Join(memDir, "events.xml")
	if _, err := os.Stat(eventsPath); os.IsNotExist(err) {
		os.WriteFile(eventsPath, []byte(emptyXML), 0644)
	}
}

func (s *ATCSkill) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	command, _ := args["command"].(string)

	switch command {
	case "analyze_tasks":
		return s.executeAnalyzeTasks(ctx, args)
	case "read_calendar":
		return s.executeReadCalendar(ctx, args)
	case "extract_keywords":
		return s.executeExtractKeywords(ctx, args)
	case "update_task":
		return s.executeUpdateTask(ctx, args)
	case "roll_over_tasks":
		return s.executeRollOverTasks(ctx, args)
	case "sync_calendar":
		return s.executeSyncCalendar(ctx, args)
	case "push_task":
		return s.executePushTask(ctx, args)
	case "list_nextcloud_tasks":
		return s.executeListNextcloudTasks(ctx, args)
	case "get_task":
		return s.executeGetTask(ctx, args)
	case "merge_task":
		return s.executeMergeTask(ctx, args)
	case "delete_task":
		return s.executeDeleteTask(ctx, args)
	default:
		return tools.ErrorResult(fmt.Sprintf("Unknown command: %s", command))
	}
}

// ----------------------------------------------------------------------------
// TOOL: analyze_tasks
// Read tasks.xml, parse the xCal schema, filter for today's VTodos,
// and mathematically calculate urgency before exporting to markdown.
// ----------------------------------------------------------------------------
func (s *ATCSkill) executeAnalyzeTasks(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	tasksPath := filepath.Join(s.workspace, "memory", "tasks.xml")
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		return tools.ErrorResult("tasks.xml file not found in ATC memory workspace. Ask the User to create one or establish a template first.")
	}

	var cal ICalendar
	if err := xml.Unmarshal(data, &cal); err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to parse tasks.xml: %v", err))
	}

	var result strings.Builder

	for _, todo := range cal.VCal.Components.VTodos {
		// Only analyze active tasks categorized for Today
		// In a real-world engine, we would parse due dates and today's actual date
		if strings.Contains(strings.ToLower(todo.Properties.Categories), "today") &&
			strings.ToUpper(todo.Properties.Status) != "COMPLETED" {

			score := s.calculateUrgency(todo)
			// Format includes the UID so the LLM knows what to pass to update_task
			result.WriteString(fmt.Sprintf("- [ ] %s [Urgency: %d] (UID: %s)\n", todo.Properties.Summary, score, todo.Properties.Uid))
		}
	}

	output := result.String()
	if output == "" {
		output = "No pending tasks found for 'Today' in tasks.xml"
	}

	return &tools.ToolResult{
		ForLLM:  output,
		ForUser: output,
	}
}

// calculateUrgency mathematically weighs the xCal properties
// to instantly prioritize the user's workload without an LLM.
func (s *ATCSkill) calculateUrgency(t VTodo) int {
	urgency := 50

	// RFC 5545 / 6321 defines priority: 1 is highest, 9 is lowest, 0 is undefined
	p := t.Properties.Priority
	if p == 1 || p == 2 {
		urgency += 40
	} else if p >= 3 && p <= 5 {
		urgency += 20
	} else if p > 5 {
		urgency += 5
	}

	// Add due date pressure
	if t.Properties.Due != "" || t.Properties.DueDate != "" {
		urgency += 10
	}

	if urgency > 100 {
		return 100
	}
	return urgency
}

// ----------------------------------------------------------------------------
// TOOL: read_calendar
// Reads memory/events.xml, parsing xCal VEvents securely via time.Parse.
// ----------------------------------------------------------------------------
func (s *ATCSkill) executeReadCalendar(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	eventsPath := filepath.Join(s.workspace, "memory", "events.xml")

	// Establish local TimeZone boundary for "Today"
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	data, err := os.ReadFile(eventsPath)
	if err != nil {
		return tools.ErrorResult("events.xml file missing or unreadable.")
	}

	var cal ICalendar
	if err := xml.Unmarshal(data, &cal); err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to parse events.xml: %v", err))
	}

	var events strings.Builder
	for _, event := range cal.VCal.Components.VEvents {
		dtStartStr := event.Properties.Dtstart
		if dtStartStr == "" {
			dtStartStr = event.Properties.DtstartDate
		}

		// Parse the RFC3339 timestamp securely.
		dtStart, err := time.Parse(time.RFC3339, dtStartStr)
		if err != nil {
			// Fallback: If it's just a raw date like "2026-02-20", try parsing that.
			dtStart, err = time.Parse("2006-01-02", dtStartStr)
			if err != nil {
				continue
			}
		}

		// Convert UTC parsing to Local TimeZone to match user's perspective.
		dtStartLocal := dtStart.Local()

		if (dtStartLocal.Equal(startOfDay) || dtStartLocal.After(startOfDay)) && dtStartLocal.Before(endOfDay) {
			events.WriteString(fmt.Sprintf("• %s - %s\n", dtStartLocal.Format("15:04"), event.Properties.Summary))
		}
	}

	output := events.String()
	if output == "" {
		output = "No calendar events found for today."
	}

	return &tools.ToolResult{
		ForLLM:  output,
		ForUser: output,
	}
}

// ----------------------------------------------------------------------------
// TOOL: extract_keywords
// Reads tasks.xml specifically for VTodos categorized as "Tomorrow".
// ----------------------------------------------------------------------------
func (s *ATCSkill) executeExtractKeywords(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	tasksPath := filepath.Join(s.workspace, "memory", "tasks.xml")
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		return tools.ErrorResult("tasks.xml file not found.")
	}

	var cal ICalendar
	if err := xml.Unmarshal(data, &cal); err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to parse tasks.xml: %v", err))
	}

	var keywords []string
	for _, todo := range cal.VCal.Components.VTodos {
		if strings.Contains(strings.ToLower(todo.Properties.Categories), "tomorrow") {
			// Remove punctuation and split words for simple extraction
			cleanText := regexp.MustCompile("[^a-zA-Z0-9 ]+").ReplaceAllString(todo.Properties.Summary, "")
			words := strings.Fields(cleanText)

			// Simple heuristic: collect words longer than 4 chars as keywords
			for _, w := range words {
				if len(w) > 4 {
					keywords = append(keywords, strings.ToLower(w))
				}
			}
		}
	}

	output := strings.Join(keywords, ", ")
	if output == "" {
		output = "No keywords extractable."
	}
	return &tools.ToolResult{
		ForLLM:  output,
		ForUser: output,
	}
}

// ----------------------------------------------------------------------------
// TOOL: update_task
// Edits tasks.xml to change the status of a specific VTodo.
// ----------------------------------------------------------------------------
func (s *ATCSkill) executeUpdateTask(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	uid, ok := args["task_uid"].(string)
	if !ok || uid == "" {
		return tools.ErrorResult("task_uid parameter is required for update_task")
	}
	newStatus, ok := args["status"].(string)
	if !ok || newStatus == "" {
		return tools.ErrorResult("status parameter is required for update_task")
	}

	tasksPath := filepath.Join(s.workspace, "memory", "tasks.xml")
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		return tools.ErrorResult("tasks.xml file not found.")
	}

	var cal ICalendar
	if err := xml.Unmarshal(data, &cal); err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to parse tasks.xml: %v", err))
	}

	found := false
	for i, todo := range cal.VCal.Components.VTodos {
		if todo.Properties.Uid == uid {
			cal.VCal.Components.VTodos[i].Properties.Status = strings.ToUpper(newStatus)
			found = true
			break
		}
	}

	if !found {
		return tools.ErrorResult(fmt.Sprintf("Task UID %s not found in XML file.", uid))
	}

	// Dump XML mapping back to string securely.
	outputBytes, err := xml.MarshalIndent(cal, "", "  ")
	if err != nil {
		return tools.ErrorResult("Failed to marshal updated task data.")
	}

	// Prepend standard XML header
	finalData := append([]byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n"), outputBytes...)
	if err := os.WriteFile(tasksPath, finalData, 0644); err != nil {
		return tools.ErrorResult("Failed to write updated XML to disk.")
	}

	msg := fmt.Sprintf("Successfully updated task %s to status %s.", uid, newStatus)
	return &tools.ToolResult{
		ForLLM:  msg,
		ForUser: msg,
	}
}

// ----------------------------------------------------------------------------
// TOOL: roll_over_tasks
// Checks tasks.xml for 'Today' tasks that weren't completed and shifts them.
// ----------------------------------------------------------------------------
func (s *ATCSkill) executeRollOverTasks(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	tasksPath := filepath.Join(s.workspace, "memory", "tasks.xml")
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		return tools.ErrorResult("tasks.xml file not found.")
	}

	var cal ICalendar
	if err := xml.Unmarshal(data, &cal); err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to parse tasks.xml: %v", err))
	}

	rolledCount := 0
	for i, todo := range cal.VCal.Components.VTodos {
		category := strings.ToLower(todo.Properties.Categories)
		status := strings.ToUpper(todo.Properties.Status)

		// If it's a "Today" task that hasn't been COMPLETED or CANCELLED
		if strings.Contains(category, "today") && status != "COMPLETED" && status != "CANCELLED" {
			// Update the category metadata.
			newCategory := strings.Replace(category, "today", "tomorrow", -1)
			if newCategory == category {
				newCategory = "tomorrow" // Fallback string rewriting
			}
			cal.VCal.Components.VTodos[i].Properties.Categories = newCategory
			rolledCount++
		}
	}

	if rolledCount > 0 {
		outputBytes, err := xml.MarshalIndent(cal, "", "  ")
		if err != nil {
			return tools.ErrorResult("Failed to marshal rolled over task data.")
		}
		finalData := append([]byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n"), outputBytes...)
		_ = os.WriteFile(tasksPath, finalData, 0644)
	}

	msg := fmt.Sprintf("Successfully rolled over %d pending 'Today' tasks into 'Tomorrow'.", rolledCount)
	return &tools.ToolResult{
		ForLLM:  msg,
		ForUser: msg,
	}
}

// ----------------------------------------------------------------------------
// TOOL: sync_calendar
// Fetches remote generic .ics subscription URLs into local xCal events.xml
// ----------------------------------------------------------------------------
func buildCalendarURL(cfg ATCCalendarConfig) string {
	return caldav.BuildCalendarURL(cfg.Host, cfg.Username)
}

func (s *ATCSkill) executeSyncCalendar(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	// Load ATC config - reads calendar_url, calendar_username, calendar_password from config.json
	atcCfg := loadATCConfig()

	// Fall back to environment variable if config is empty
	calendarURL := buildCalendarURL(atcCfg)
	if atcCfg.Host == "" {
		calendarURL = os.Getenv("ATC_CALENDAR_URL")
	}
	if calendarURL == "" {
		return tools.ErrorResult("No host configured. Set host in config.json under tools.nextcloud, or set the ATC_CALENDAR_URL environment variable.")
	}

	lines, err := fetchICS(calendarURL, atcCfg.Username, atcCfg.Password)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to fetch external calendar: %v", err))
	}

	cal := parseICS(lines)
	if cal == nil || len(cal.VCal.Components.VEvents) == 0 {
		return tools.ErrorResult("Failed to parse external iCal data or no events found.")
	}

	// Hardcode the absolute workspace path since the LLM executor context might be running under 'monitor' or 'chief'
	eventsPath := filepath.Join("workspaces", "atc", "memory", "events.xml")
	outputBytes, err := xml.MarshalIndent(cal, "", "  ")
	if err != nil {
		return tools.ErrorResult("Failed to marshal synced calendar data.")
	}

	finalData := append([]byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n"), outputBytes...)
	if err := os.WriteFile(eventsPath, finalData, 0644); err != nil {
		return tools.ErrorResult("Failed to locally save synced events.xml.")
	}

	count := len(cal.VCal.Components.VEvents)
	msg := fmt.Sprintf("Successfully synced %d events from Nextcloud (%s). Saved to events.xml.", count, calendarURL)
	return &tools.ToolResult{ForLLM: msg, ForUser: msg}
}

// ----------------------------------------------------------------------------
// TOOL: push_task
// Writes a new VTODO task to Nextcloud CalDAV via HTTP PUT.
// ----------------------------------------------------------------------------
func (s *ATCSkill) executePushTask(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	summary, _ := args["summary"].(string)
	if summary == "" {
		return tools.ErrorResult("summary parameter is required for push_task")
	}

	atcCfg := loadATCConfig()
	if atcCfg.Host == "" {
		return tools.ErrorResult("host not configured in config.json tools.nextcloud")
	}

	// Build TaskOptions from optional LLM args
	opts := TaskOptions{
		Due:   getString(args, "due"),
		Start: getString(args, "start"),
		Notes: getString(args, "notes"),
	}
	if p, ok := args["priority"].(float64); ok {
		opts.Priority = int(p)
	}

	taskUID := fmt.Sprintf("atc-task-%d", time.Now().UnixNano())

	if err := pushTaskToCalDAV(atcCfg, taskUID, summary, opts); err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to push task to Nextcloud: %v", err))
	}

	msg := fmt.Sprintf("✅ Task '%s' successfully pushed to your Nextcloud Tasks (UID: %s).", summary, taskUID)
	return &tools.ToolResult{
		ForLLM:  msg,
		ForUser: msg,
	}
}

// getString safely extracts a string from args
func getString(args map[string]interface{}, key string) string {
	v, _ := args[key].(string)
	return v
}

// ----------------------------------------------------------------------------
// TOOL: list_nextcloud_tasks
// Does a CalDAV PROPFIND to list all task hrefs in Nextcloud tasks/ collection.
// ----------------------------------------------------------------------------
func (s *ATCSkill) executeListNextcloudTasks(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	atcCfg := loadATCConfig()
	if atcCfg.Host == "" {
		return tools.ErrorResult("host not configured in config.json tools.nextcloud")
	}

	hrefs, err := listNextcloudTasks(atcCfg)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to list Nextcloud tasks: %v", err))
	}
	if len(hrefs) == 0 {
		msg := "No tasks found in your Nextcloud Tasks collection."
		return &tools.ToolResult{ForLLM: msg, ForUser: msg}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d tasks:\n", len(hrefs)))
	for _, h := range hrefs {
		sb.WriteString("  - " + h + "\n")
	}
	out := sb.String()
	return &tools.ToolResult{ForLLM: out, ForUser: out}
}

// ----------------------------------------------------------------------------
// TOOL: delete_task
// Deletes a specific task from Nextcloud CalDAV by its href path.
// ----------------------------------------------------------------------------
func (s *ATCSkill) executeDeleteTask(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	href := getString(args, "task_href")
	if href == "" {
		return tools.ErrorResult("task_href is required. Use list_nextcloud_tasks first to get the href paths.")
	}

	atcCfg := loadATCConfig()
	if atcCfg.Host == "" {
		return tools.ErrorResult("host not configured in config.json tools.nextcloud")
	}

	if err := deleteTaskFromCalDAV(atcCfg, href); err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to delete task: %v", err))
	}

	msg := fmt.Sprintf("🗑️ Task deleted: %s", href)
	return &tools.ToolResult{ForLLM: msg, ForUser: msg}
}

// ----------------------------------------------------------------------------
// TOOL: get_task
// Fetches a single task from Nextcloud by its CalDAV href and shows its fields.
// ----------------------------------------------------------------------------
func (s *ATCSkill) executeGetTask(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	href := getString(args, "task_href")
	if href == "" {
		return tools.ErrorResult("task_href is required. Use list_nextcloud_tasks to get the href paths.")
	}
	atcCfg := loadATCConfig()
	fields, err := getTaskFromCalDAV(atcCfg, href)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to get task: %v", err))
	}
	var sb strings.Builder
	sb.WriteString("Task details:\n")
	for _, k := range []string{"SUMMARY", "UID", "STATUS", "PRIORITY", "DUE", "DTSTART", "DESCRIPTION", "LOCATION", "URL", "PERCENT-COMPLETE"} {
		if v, ok := fields[k]; ok && v != "" {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
	}
	out := sb.String()
	return &tools.ToolResult{ForLLM: out, ForUser: out}
}

// ----------------------------------------------------------------------------
// TOOL: merge_task
// Fetches an existing task by href, merges updated fields, and writes it back.
// ----------------------------------------------------------------------------
func (s *ATCSkill) executeMergeTask(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	href := getString(args, "task_href")
	if href == "" {
		return tools.ErrorResult("task_href is required. Use list_nextcloud_tasks to get the href paths.")
	}
	atcCfg := loadATCConfig()
	opts := TaskOptions{
		Due:      getString(args, "due"),
		Start:    getString(args, "start"),
		Notes:    getString(args, "notes"),
		Location: getString(args, "location"),
	}
	if p, ok := args["priority"].(float64); ok {
		opts.Priority = int(p)
	}
	newSummary := getString(args, "summary")

	if err := mergeTaskOnCalDAV(atcCfg, href, opts, newSummary); err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to merge task: %v", err))
	}

	msg := fmt.Sprintf("✏️ Task updated: %s", href)
	return &tools.ToolResult{ForLLM: msg, ForUser: msg}
}
