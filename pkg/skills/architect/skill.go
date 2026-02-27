package architect

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jony/son-of-anthon/pkg/skills/caldav"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ArchitectConfig holds the credentials and endpoints from config.json.
type ArchitectConfig struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
	Timeout  int    `json:"timeout_seconds"`
}

// ArchitectSkill is the subagent responsible for managing recurring life admin via Nextcloud CalDAV.
type ArchitectSkill struct {
	workspace string
}

func NewSkill() *ArchitectSkill {
	return &ArchitectSkill{}
}

func (s *ArchitectSkill) Name() string {
	return "architect"
}

func (s *ArchitectSkill) Description() string {
	return "Life Architect (Sage): Manages your recurring life admin (rent, medicine) via Nextcloud. Can sync deadlines, and natively create complex recurring CalDAV VTODOs."
}

func (s *ArchitectSkill) SetWorkspace(workspacePath string) {
	s.workspace = workspacePath
	// Ensure memory directory exists for deadlines-today.md
	memPath := filepath.Join(workspacePath, "memory")
	_ = os.MkdirAll(memPath, 0755)
	s.initWorkspace()
}

func (s *ArchitectSkill) initWorkspace() {
	if s.workspace == "" {
		return
	}
	identityPath := filepath.Join(s.workspace, "IDENTITY.md")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		identityContent := `# Life Architect - Identity

- **Name:** Sage
- **Creature:** Organized planner with clipboard and calendar 📋
- **Vibe:** "Heads up, this is due soon" (proactive, never pushy)
- **Emoji:** 🏗️
- **Catchphrase:** "Keeping track so you don't have to..."
`
		os.WriteFile(identityPath, []byte(identityContent), 0644)
	}

	heartbeatPath := filepath.Join(s.workspace, "HEARTBEAT.md")
	if _, err := os.Stat(heartbeatPath); os.IsNotExist(err) {
		heartbeatContent := `# HEARTBEAT.md

# Keep this file empty (or with only comments) to skip heartbeat API calls.
`
		os.WriteFile(heartbeatPath, []byte(heartbeatContent), 0644)
	}
}

func loadArchitectConfig() ArchitectConfig {
	var cfg struct {
		Tools struct {
			Nextcloud ArchitectConfig `json:"nextcloud"`
		} `json:"tools"`
	}
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".picoclaw", "config.json")
	if envPath := os.Getenv("PERSONAL_OS_CONFIG"); envPath != "" {
		configPath = envPath
	}

	data, err := os.ReadFile(configPath)
	if err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	return cfg.Tools.Nextcloud
}

func (s *ArchitectSkill) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command to execute",
				"enum":        []string{"sync_deadlines", "create_task", "delete_task"},
			},
			"uuid": map[string]interface{}{
				"type":        "string",
				"description": "UUID of the task to delete (from [task_id: ...] in the dashboard). Provide either uuid OR title, not both.",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Title/name of the task to delete (e.g. 'Medicine Order'). Will delete ALL tasks matching this name. Provide either title OR uuid, not both. Used in both create_task and delete_task.",
			},
			"task_type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"recurring", "onetime"},
				"description": "Whether this is 'recurring' (VTODO) or 'onetime' (VEVENT). Used in create_task.",
			},
			"interval_days": map[string]interface{}{
				"type":        "integer",
				"description": "If recurring: How often in days (e.g. 30). This auto-generates RRULE. Leave empty for onetime. Used in create_task.",
			},
			"target_date": map[string]interface{}{
				"type":        "string",
				"description": "If recurring: FIRST due date. If onetime: deadline block date. Format: YYYY-MM-DD. Used in create_task.",
			},
		},
		"required": []string{"command"},
	}
}

func (s *ArchitectSkill) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	command, _ := args["command"].(string)

	switch command {
	case "sync_deadlines":
		return s.executeSyncDeadlines(ctx, args)
	case "create_task":
		return s.executeCreateTask(ctx, args)
	case "delete_task":
		return s.executeDeleteTask(ctx, args)
	default:
		return tools.ErrorResult(fmt.Sprintf("Unknown command: %s", command))
	}
}

func buildTasksURL(cfg ArchitectConfig) string {
	return caldav.BuildTasksURL(cfg.Host, cfg.Username)
}

func buildCalendarURL(cfg ArchitectConfig) string {
	return caldav.BuildCalendarURL(cfg.Host, cfg.Username)
}

func (s *ArchitectSkill) executeSyncDeadlines(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	cfg := loadArchitectConfig()
	loc, err := time.LoadLocation("Asia/Dhaka")
	if err != nil {
		return tools.ErrorResult("Failed to load timezone Asia/Dhaka")
	}
	now := time.Now().In(loc)

	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}

	// 1. Collect .ics hrefs from VTODOs (tasks calendar)
	tasksURL := buildTasksURL(cfg)
	taskHrefs, _ := propfindHrefs(client, tasksURL, cfg.Username, cfg.Password)

	// 2. Collect .ics hrefs from VEVENTs (personal calendar — one-time deadlines)
	calBase := buildCalendarURL(cfg)
	calHrefs, _ := propfindHrefs(client, calBase, cfg.Username, cfg.Password)

	allHrefs := append(taskHrefs, calHrefs...)

	var urgent []string
	var upcoming []string
	var completed []string

	for _, href := range allHrefs {
		parts := strings.Split(href, "/")
		filename := parts[len(parts)-1]
		uuid := strings.TrimSuffix(filename, ".ics")

		fields, err := s.getTaskFromCalDAV(cfg, href)
		if err != nil {
			continue
		}

		summary := fields["SUMMARY"]
		if summary == "" {
			continue
		}
		status := fields["STATUS"]
		pct := fields["PERCENT-COMPLETE"]
		dueStr := fields["DUE"]
		if dueStr == "" {
			dueStr = fields["DTSTART"]
		}

		isCompleted := status == "COMPLETED" || pct == "100"
		if isCompleted {
			completed = append(completed, fmt.Sprintf("- [task_id: %s] %s: Marked completed on CalDAV. *Action: Log to MEMORY.md and celebrate.*", uuid, summary))
			continue
		}

		if dueStr != "" && len(dueStr) >= 8 {
			dueOnly := dueStr[:8]
			dueDate, parseErr := time.ParseInLocation("20060102", dueOnly, loc)
			if parseErr == nil {
				daysDiff := int(dueDate.Sub(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)).Hours() / 24)
				if daysDiff < 0 {
					// OVERDUE — embed ISO at T00:00 so Chief always flags it
					urgent = append(urgent, fmt.Sprintf("- [task_id: %s] %s: OVERDUE by %d days %sT00:00. *Action: Flag as overdue.*", uuid, summary, -daysDiff, dueDate.Format("2006-01-02")))
				} else if daysDiff == 0 {
					// DUE TODAY — embed ISO at T09:00 (morning, within Chief's 2h window from 9am)
					urgent = append(urgent, fmt.Sprintf("- [task_id: %s] %s: DUE TODAY %sT09:00. *Action: Send urgent reminder.*", uuid, summary, dueDate.Format("2006-01-02")))
				} else if daysDiff <= 7 {
					upcoming = append(upcoming, fmt.Sprintf("- [task_id: %s] %s: Due in %d days (%s). *Action: Monitor, no reminder needed yet.*", uuid, summary, daysDiff, dueDate.Format("Jan 02")))
				}
			}
		}
	}

	var md strings.Builder
	md.WriteString(fmt.Sprintf("# Life Admin Status - %s\n\n", now.Format("2006-01-02")))

	md.WriteString("## 🚨 URGENT (Due Today / Overdue)\n")
	if len(urgent) > 0 {
		for _, u := range urgent {
			md.WriteString(u + "\n")
		}
	} else {
		md.WriteString("- *No urgent tasks*\n")
	}
	md.WriteString("\n")

	md.WriteString("## ⏳ UPCOMING (Next 7 Days)\n")
	if len(upcoming) > 0 {
		for _, u := range upcoming {
			md.WriteString(u + "\n")
		}
	} else {
		md.WriteString("- *No upcoming tasks*\n")
	}
	md.WriteString("\n")

	md.WriteString("## 📋 RECENTLY COMPLETED (Feedback Loop)\n")
	if len(completed) > 0 {
		for _, c := range completed {
			md.WriteString(c + "\n")
		}
	} else {
		md.WriteString("- *No recent completions*\n")
	}

	memDir := filepath.Join(s.workspace, "memory")
	_ = os.MkdirAll(memDir, 0755)
	tmpFile := filepath.Join(memDir, "deadlines-today.md.tmp")
	finalFile := filepath.Join(memDir, "deadlines-today.md")

	err = os.WriteFile(tmpFile, []byte(md.String()), 0644)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to write temporary markdown: %v", err))
	}
	err = os.Rename(tmpFile, finalFile)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Atomic rename failed for deadlines-today.md: %v", err))
	}

	return &tools.ToolResult{
		ForLLM:  md.String(), // Full dashboard with UUIDs — LLM can parse and act on them
		ForUser: "✅ Synced deadlines. Dashboard updated at memory/deadlines-today.md",
	}
}

// propfindHrefs issues a CalDAV PROPFIND Depth:1 and returns all .ics hrefs.
func propfindHrefs(client *http.Client, calURL, username, password string) ([]string, error) {
	req, err := http.NewRequest("PROPFIND", calURL,
		strings.NewReader(`<?xml version="1.0"?><propfind xmlns="DAV:"><prop><getetag/></prop></propfind>`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	req.SetBasicAuth(username, password)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var hrefs []string
	for _, line := range strings.Split(string(body), "<") {
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "d:href>") || strings.HasPrefix(lower, "href>") {
			val := strings.SplitN(line, ">", 2)
			if len(val) == 2 && strings.HasSuffix(strings.TrimSpace(val[1]), ".ics") {
				hrefs = append(hrefs, strings.TrimSpace(val[1]))
			}
		}
	}
	return hrefs, nil
}

func (s *ArchitectSkill) executeDeleteTask(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	cfg := loadArchitectConfig()
	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}

	// --- Path A: delete by explicit UUID ---
	uuid, _ := args["uuid"].(string)
	if uuid != "" && strings.Contains(uuid, "-") && len(uuid) > 30 {
		return s.deleteByUUID(client, cfg, uuid)
	}

	// --- Path B: delete by title (SUMMARY match) ---
	title, _ := args["title"].(string)
	if title != "" {
		tasksURL := buildTasksURL(cfg)
		hrefs, err := propfindHrefs(client, tasksURL, cfg.Username, cfg.Password)
		if err != nil {
			return tools.ErrorResult(fmt.Sprintf("PROPFIND failed: %v", err))
		}

		deleted := 0
		var errs []string
		for _, href := range hrefs {
			fields, err := s.getTaskFromCalDAV(cfg, href)
			if err != nil {
				continue
			}
			if strings.EqualFold(fields["SUMMARY"], title) {
				parts := strings.Split(href, "/")
				uuidFromHref := strings.TrimSuffix(parts[len(parts)-1], ".ics")
				res := s.deleteByUUID(client, cfg, uuidFromHref)
				if res.IsError {
					errs = append(errs, res.ForLLM)
				} else {
					deleted++
				}
			}
		}
		if len(errs) > 0 {
			return tools.ErrorResult(fmt.Sprintf("Deleted %d, but %d errors: %s", deleted, len(errs), strings.Join(errs, "; ")))
		}
		if deleted == 0 {
			return tools.ErrorResult(fmt.Sprintf("No tasks named '%s' found in Nextcloud Tasks calendar.", title))
		}
		return tools.UserResult(fmt.Sprintf("✅ Deleted %d task(s) named '%s' from Nextcloud CalDAV.", deleted, title))
	}

	return tools.ErrorResult("Provide either 'uuid' (exact task ID) or 'title' (task name) to delete.")
}

func (s *ArchitectSkill) deleteByUUID(client *http.Client, cfg ArchitectConfig, uuid string) *tools.ToolResult {
	tasksURL := buildTasksURL(cfg)
	url := tasksURL + uuid + ".ics"
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("DELETE request failed: %v", err))
	}
	req.SetBasicAuth(cfg.Username, cfg.Password)
	resp, err := client.Do(req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("HTTP DELETE failed: %v", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return tools.UserResult(fmt.Sprintf("✅ Task %s deleted from Nextcloud CalDAV.", uuid))
	}
	body, _ := io.ReadAll(resp.Body)
	return tools.ErrorResult(fmt.Sprintf("Nextcloud rejected DELETE. Status: %d, Response: %s", resp.StatusCode, string(body)))
}

func (s *ArchitectSkill) getTaskFromCalDAV(cfg ArchitectConfig, href string) (map[string]string, error) {
	tasksURL := buildTasksURL(cfg)
	idx := strings.Index(tasksURL, "/remote.php")
	var fullURL string
	if idx > 0 && !strings.HasPrefix(href, "http") {
		fullURL = tasksURL[:idx] + href
	} else {
		fullURL = href
	}
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cfg.Username, cfg.Password)

	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	fields := map[string]string{}
	// Normalize line endings and unfold
	raw := strings.ReplaceAll(string(body), "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\n ", "")
	raw = strings.ReplaceAll(raw, "\n\t", "")

	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(strings.SplitN(parts[0], ";", 2)[0]))
		val := strings.TrimSpace(parts[1])
		switch key {
		case "SUMMARY", "STATUS", "PERCENT-COMPLETE", "COMPLETED", "LAST-MODIFIED", "DUE", "DTSTART":
			// Unescape
			val = strings.ReplaceAll(val, "\\,", ",")
			val = strings.ReplaceAll(val, "\\;", ";")
			val = strings.ReplaceAll(val, "\\n", "\n")
			fields[key] = val
		}
	}
	return fields, nil
}

func (s *ArchitectSkill) executeCreateTask(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	title, ok := args["title"].(string)
	if !ok {
		return tools.ErrorResult("Missing 'title'")
	}
	taskType, ok := args["task_type"].(string)
	if !ok {
		return tools.ErrorResult("Missing 'task_type'")
	}
	targetDateStr, ok := args["target_date"].(string)
	if !ok {
		return tools.ErrorResult("Missing 'target_date'")
	}

	loc, err := time.LoadLocation("Asia/Dhaka")
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to load timezone Asia/Dhaka: %v", err))
	}

	targetDate, err := time.ParseInLocation("2006-01-02", targetDateStr, loc)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Invalid target_date format: %v", err))
	}

	cfg := loadArchitectConfig()

	nowUTC := time.Now().UTC().Format("20060102T150405Z")
	uuid := generateUUID()
	var pb strings.Builder

	pb.WriteString("BEGIN:VCALENDAR\r\n")
	pb.WriteString("VERSION:2.0\r\n")
	pb.WriteString("PRODID:-//Son of Anthon//Life Architect Sage//EN\r\n")

	if taskType == "recurring" {
		intervalFloat, ok := args["interval_days"].(float64)
		if !ok {
			return tools.ErrorResult("Missing 'interval_days' for recurring task")
		}
		interval := int(intervalFloat)

		pb.WriteString("BEGIN:VTODO\r\n")
		pb.WriteString(fmt.Sprintf("UID:%s\r\n", uuid))
		pb.WriteString(fmt.Sprintf("DTSTAMP:%s\r\n", nowUTC))
		pb.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", title))
		pb.WriteString("STATUS:NEEDS-ACTION\r\n")

		dateOnly := targetDate.Format("20060102")
		pb.WriteString(fmt.Sprintf("DTSTART;VALUE=DATE:%s\r\n", dateOnly))
		pb.WriteString(fmt.Sprintf("DUE;VALUE=DATE:%s\r\n", dateOnly))
		pb.WriteString(fmt.Sprintf("RRULE:FREQ=DAILY;INTERVAL=%d\r\n", interval))
		pb.WriteString("END:VTODO\r\n")

	} else if taskType == "onetime" {
		pb.WriteString("BEGIN:VEVENT\r\n")
		pb.WriteString(fmt.Sprintf("UID:%s\r\n", uuid))
		pb.WriteString(fmt.Sprintf("DTSTAMP:%s\r\n", nowUTC))
		pb.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", title))

		dateOnly := targetDate.Format("20060102")
		pb.WriteString(fmt.Sprintf("DTSTART;VALUE=DATE:%s\r\n", dateOnly))
		// End date is exclusive for VEVENT
		nextDay := targetDate.AddDate(0, 0, 1).Format("20060102")
		pb.WriteString(fmt.Sprintf("DTEND;VALUE=DATE:%s\r\n", nextDay))
		pb.WriteString("TRANSP:TRANSPARENT\r\n")
		pb.WriteString("END:VEVENT\r\n")
	} else {
		return tools.ErrorResult("Unknown task_type (must be recurring or onetime)")
	}

	pb.WriteString("END:VCALENDAR\r\n")
	payloadStr := pb.String()

	var url string
	if taskType == "recurring" {
		tasksURL := buildTasksURL(cfg)
		url = tasksURL + uuid + ".ics"
	} else {
		calBase := buildCalendarURL(cfg)
		url = calBase + uuid + ".ics"
	}

	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(payloadStr))
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("HTTP request creation failed: %v", err))
	}
	req.SetBasicAuth(cfg.Username, cfg.Password)
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")

	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}

	resp, err := client.Do(req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("HTTP PUT failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return tools.ErrorResult(fmt.Sprintf("Nextcloud rejected CalDAV push. Status: %d, Response: %s", resp.StatusCode, string(bodyBytes)))
	}

	return tools.UserResult(fmt.Sprintf("Successfully pushed %s '%s' to Nextcloud CalDAV (UUID: %s)", taskType, title, uuid))
}

// generateUUID returns a standard UUID using crypto/rand required by CalDAV RFC 5545
func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
