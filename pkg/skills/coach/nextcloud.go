package coach

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jony/son-of-anthon/pkg/skills/caldav"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ----------------------------------------------------------------------------
// Nextcloud Deck Sync
// ----------------------------------------------------------------------------

func (s *CoachSkill) executeSyncDeck(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	cfg := loadCoachConfig()
	if cfg.Host == "" {
		return tools.ErrorResult("Nextcloud not configured. Set tools.nextcloud in config.json")
	}

	if s.store == nil {
		return tools.ErrorResult("Coach store not initialized")
	}

	// Get all active courses
	courses, err := s.store.GetActiveCourses()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Error getting courses: %v", err))
	}

	deckURL := caldav.BuildDeckURL(cfg.Host)
	apiURL := strings.TrimRight(deckURL, "/") + "/api/v1.0"

	// Get boards to find or create "Learning" board
	boardID, err := findOrCreateDeckBoard(apiURL, cfg.Username, cfg.Password, "Learning")
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Deck error: %v", err))
	}

	synced := 0
	for _, course := range courses {
		// Find or create card for this course
		cardID, err := findOrCreateDeckCard(apiURL, cfg.Username, cfg.Password, boardID, course.Name, string(course.Type), course.TotalUnits, course.Completed)
		if err != nil {
			continue
		}

		// Update card description with progress
		desc := fmt.Sprintf("**Type:** %s\n**Progress:** %d/%d (%d%%)\n**Pace:** %.1f units/day",
			course.Type, course.Completed, course.TotalUnits, (course.Completed*100)/course.TotalUnits, course.Pace7Day)

		err = updateDeckCard(apiURL, cfg.Username, cfg.Password, boardID, cardID, course.Name, desc)
		if err == nil {
			synced++
		}
	}

	return tools.UserResult(fmt.Sprintf("✅ Synced %d courses to Nextcloud Deck", synced))
}

func findOrCreateDeckBoard(apiURL, username, password, boardName string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Try to find existing board
	req, _ := http.NewRequest("GET", apiURL+"/boards", nil)
	req.SetBasicAuth(username, password)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var boards []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&boards)
		for _, b := range boards {
			if b["title"] == boardName {
				return fmt.Sprintf("%.0f", b["id"].(float64)), nil
			}
		}
	}

	// Create new board
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]string{"title": boardName})
	req, _ = http.NewRequest("POST", apiURL+"/boards", &buf)
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var newBoard map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&newBoard)
	return fmt.Sprintf("%.0f", newBoard["id"].(float64)), nil
}

func findOrCreateDeckCard(apiURL, username, password, boardID, title, courseType string, total, completed int) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Get stacks to find "Active" stack
	req, _ := http.NewRequest("GET", apiURL+"/boards/"+boardID+"/stacks", nil)
	req.SetBasicAuth(username, password)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var stacks []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&stacks)

	stackID := ""
	for _, st := range stacks {
		title := st["title"].(string)
		if title == "Active" || title == "📚 Active" {
			stackID = fmt.Sprintf("%.0f", st["id"].(float64))
			break
		}
	}

	if stackID == "" && len(stacks) > 0 {
		stackID = fmt.Sprintf("%.0f", stacks[0]["id"].(float64))
	}

	if stackID == "" {
		return "", fmt.Errorf("no stack found")
	}

	// Try to find existing card
	req, _ = http.NewRequest("GET", apiURL+"/stacks/"+stackID+"/cards", nil)
	req.SetBasicAuth(username, password)
	resp, err = client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var cards []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&cards)
		for _, c := range cards {
			if c["title"] == title {
				return fmt.Sprintf("%.0f", c["id"].(float64)), nil
			}
		}
	}

	// Create new card
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]interface{}{
		"title":       title,
		"description": fmt.Sprintf("Type: %s\nTotal: %d", courseType, total),
	})
	req, _ = http.NewRequest("POST", apiURL+"/stacks/"+stackID+"/cards", &buf)
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var newCard map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&newCard)
	return fmt.Sprintf("%.0f", newCard["id"].(float64)), nil
}

func updateDeckCard(apiURL, username, password, boardID, cardID, title, description string) error {
	client := &http.Client{Timeout: 10 * time.Second}

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]interface{}{
		"title":       title,
		"description": description,
	})
	req, _ := http.NewRequest("PUT", apiURL+"/boards/"+boardID+"/cards/"+cardID, &buf)
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API returned %d", resp.StatusCode)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Nextcloud Tasks (CalDAV) Sync
// ----------------------------------------------------------------------------

func (s *CoachSkill) executeSyncTasks(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	cfg := loadCoachConfig()
	if cfg.Host == "" {
		return tools.ErrorResult("Nextcloud not configured. Set tools.nextcloud in config.json")
	}

	if s.store == nil {
		return tools.ErrorResult("Store not initialized")
	}

	courses, err := s.store.GetActiveCourses()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Error: %v", err))
	}

	synced := 0

	for _, course := range courses {
		units, err := s.store.GetCourseUnits(course.ID)
		if err != nil {
			continue
		}

		for _, unit := range units {
			uid := fmt.Sprintf("coach-%s-%s", course.ID, unit.ID)
			summary := fmt.Sprintf("[%s] %s", course.Name, unit.Title)
			status := "NEEDS-ACTION"
			if unit.Status == "completed" {
				status = "COMPLETED"
			}

			err := pushTaskToCalDAV(cfg, uid, summary, TaskOptions{
				Description: fmt.Sprintf("course:%s\nindex:%d\ntype:%s", course.Name, unit.Index, course.Type),
				Status:      status,
			})
			if err == nil {
				synced++
			}
		}
	}

	return &tools.ToolResult{
		ForUser: fmt.Sprintf("✅ Synced %d units to Nextcloud Tasks", synced),
		ForLLM:  fmt.Sprintf("Synced %d tasks", synced),
	}
}

type TaskOptions struct {
	Description string
	Status      string
	DueDate     *time.Time
}

func pushTaskToCalDAV(cfg CoachConfig, uid, summary string, opts TaskOptions) error {
	tasksURL := caldav.BuildTasksURL(cfg.Host, cfg.Username)
	fullURL := tasksURL + uid + ".ics"

	now := time.Now()
	due := ""
	if opts.DueDate != nil {
		due = fmt.Sprintf("DUE:%s\r\n", opts.DueDate.Format("20060102T150000Z"))
	}

	ics := fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Son of Anthon//Coach//EN
BEGIN:VTODO
UID:%s@son-of-anthon
DTSTAMP:%s
SUMMARY:%s
%sSTATUS:%s
DESCRIPTION:%s
END:VTODO
END:VCALENDAR
`, uid, now.Format("20060102T150000Z"), summary, due, opts.Status, opts.Description)

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("PUT", fullURL, strings.NewReader(ics))
	req.SetBasicAuth(cfg.Username, cfg.Password)
	req.Header.Set("Content-Type", "text/calendar")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ----------------------------------------------------------------------------
// Nextcloud Calendar Sync
// ----------------------------------------------------------------------------

func (s *CoachSkill) executeSyncCalendar(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	cfg := loadCoachConfig()
	if cfg.Host == "" {
		return tools.ErrorResult("Nextcloud not configured")
	}

	if s.store == nil {
		return tools.ErrorResult("Store not initialized")
	}

	// Get sessions from last 7 days
	stats, err := s.store.GetWeeklyStats()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Error: %v", err))
	}

	synced := 0

	// Create a sample VEVENT for the week
	weekAgo := time.Now().AddDate(0, 0, -7)
	uid := fmt.Sprintf("week-%d", weekAgo.Unix())
	summary := fmt.Sprintf("Learning: %d units this week", stats["units_completed"].(int))

	err = pushEventToCalDAV(cfg, uid, summary, weekAgo, 60, "")
	if err == nil {
		synced = 1
	}

	return &tools.ToolResult{
		ForUser: fmt.Sprintf("✅ Synced %d sessions to Nextcloud Calendar", synced),
		ForLLM:  fmt.Sprintf("Synced %d events", synced),
	}
}

func pushEventToCalDAV(cfg CoachConfig, uid, summary string, start time.Time, durationMinutes int, description string) error {
	calURL := caldav.BuildCalendarURL(cfg.Host, cfg.Username) + "learning-sessions"
	fullURL := calURL + uid + ".ics"

	end := start.Add(time.Duration(durationMinutes) * time.Minute)

	ics := fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Son of Anthon//Coach//EN
BEGIN:VEVENT
UID:%s@son-of-anthon
DTSTAMP:%s
DTSTART:%s
DTEND:%s
SUMMARY:%s
DESCRIPTION:%s
CATEGORIES:learning
END:VEVENT
END:VCALENDAR
`, uid, start.Format("20060102T150000Z"), start.Format("20060102T150000Z"), end.Format("20060102T150000Z"), summary, description)

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("PUT", fullURL, strings.NewReader(ics))
	req.SetBasicAuth(cfg.Username, cfg.Password)
	req.Header.Set("Content-Type", "text/calendar")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
