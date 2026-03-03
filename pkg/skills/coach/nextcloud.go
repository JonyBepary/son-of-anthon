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

	courses, err := s.store.GetActiveCourses()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Error getting courses: %v", err))
	}

	deckURL := caldav.BuildDeckURL(cfg.Host)
	apiURL := strings.TrimRight(deckURL, "/")

	boardID, err := findOrCreateDeckBoard(apiURL, cfg.Username, cfg.Password, "Learning")
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Deck error: %v", err))
	}

	// Ensure 3 stacks exist
	stacks, err := ensureKanbanStacks(apiURL, cfg.Username, cfg.Password, boardID)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Stack error: %v", err))
	}

	synced := 0
	for _, course := range courses {
		progress := 0
		if course.TotalUnits > 0 {
			progress = (course.Completed * 100) / course.TotalUnits
		}

		// Determine target stack
		targetStack := "Want To Learn"
		if progress >= 100 {
			targetStack = "Completed"
		} else if progress > 0 {
			targetStack = "In Progress"
		}

		stackID := stacks[targetStack]
		cardID, err := findOrCreateDeckCardInStack(apiURL, cfg.Username, cfg.Password, boardID, stackID, course.Name)
		if err != nil {
			continue
		}

		// Build progress visualization
		desc := buildProgressDescription(*course, s.store)
		labels := buildCourseLabels(*course)

		err = updateDeckCardWithLabels(apiURL, cfg.Username, cfg.Password, boardID, stackID, cardID, course.Name, desc, labels)
		if err == nil {
			synced++
		}
	}

	return tools.UserResult(fmt.Sprintf("✅ Synced %d courses to Nextcloud Deck", synced))
}

func findOrCreateDeckBoard(apiURL, username, password, boardName string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Try to find existing board
	req, _ := http.NewRequest("GET", apiURL+"/boards", nil)
	req.SetBasicAuth(username, password)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("connection failed: %v (check if Nextcloud Deck app is installed)", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API returned status %d: %s (Deck app may not be installed)", resp.StatusCode, string(body))
	}

	var boards []map[string]interface{}
	if err := json.Unmarshal(body, &boards); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	for _, b := range boards {
		if b["title"] == boardName && b["id"] != nil {
			return fmt.Sprintf("%.0f", b["id"].(float64)), nil
		}
	}

	// Create new board
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]string{"title": boardName, "color": "31CC7C"})
	req, _ = http.NewRequest("POST", apiURL+"/boards", &buf)
	req.SetBasicAuth(username, password)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		return "", fmt.Errorf("connection failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to create board: status %d, response: %s", resp.StatusCode, string(respBody))
	}

	var newBoard map[string]interface{}
	if err := json.Unmarshal(respBody, &newBoard); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}
	if newBoard == nil || newBoard["id"] == nil {
		return "", fmt.Errorf("board created but no ID returned")
	}
	return fmt.Sprintf("%.0f", newBoard["id"].(float64)), nil
}

func findOrCreateDeckCard(apiURL, username, password, boardID, title, courseType string, total, completed int) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Get stacks to find "Active" stack
	req, _ := http.NewRequest("GET", apiURL+"/boards/"+boardID+"/stacks", nil)
	req.SetBasicAuth(username, password)
	req.Header.Set("OCS-APIRequest", "true")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var stacks []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&stacks)

	stackID := ""
	for _, st := range stacks {
		if st["title"] == nil || st["id"] == nil {
			continue
		}
		title := st["title"].(string)
		if title == "Active" || title == "📚 Active" {
			stackID = fmt.Sprintf("%.0f", st["id"].(float64))
			break
		}
	}

	if stackID == "" && len(stacks) > 0 {
		for _, s := range stacks {
			if s["id"] != nil {
				stackID = fmt.Sprintf("%.0f", s["id"].(float64))
				break
			}
		}
	}

	if stackID == "" {
		return "", fmt.Errorf("no stack found")
	}

	// Try to find existing card
	req, _ = http.NewRequest("GET", apiURL+"/stacks/"+stackID+"/cards", nil)
	req.SetBasicAuth(username, password)
	req.Header.Set("OCS-APIRequest", "true")
	resp, err = client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var cards []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&cards)
		for _, c := range cards {
			if c["title"] == title && c["id"] != nil {
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
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to create card: status %d", resp.StatusCode)
	}

	var newCard map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&newCard)
	if newCard == nil || newCard["id"] == nil {
		return "", fmt.Errorf("card created but no ID returned")
	}
	return fmt.Sprintf("%.0f", newCard["id"].(float64)), nil
}

func ensureKanbanStacks(apiURL, username, password, boardID string) (map[string]string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, _ := http.NewRequest("GET", apiURL+"/boards/"+boardID+"/stacks", nil)
	req.SetBasicAuth(username, password)
	req.Header.Set("OCS-APIRequest", "true")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var stacks []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&stacks)

	stackMap := make(map[string]string)
	needed := []struct{ name, color string }{
		{"Want To Learn", "0800fd"},
		{"In Progress", "ff6f00"},
		{"Completed", "31cc7c"},
	}

	for _, n := range needed {
		found := false
		for _, st := range stacks {
			if st["title"] == n.name && st["id"] != nil {
				stackMap[n.name] = fmt.Sprintf("%.0f", st["id"].(float64))
				found = true
				break
			}
		}
		if !found {
			id, err := createStack(apiURL, username, password, boardID, n.name, n.color)
			if err != nil {
				return nil, err
			}
			stackMap[n.name] = id
		}
	}
	return stackMap, nil
}

func createStack(apiURL, username, password, boardID, title, color string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]interface{}{"title": title, "color": color, "order": 999})

	req, _ := http.NewRequest("POST", apiURL+"/boards/"+boardID+"/stacks", &buf)
	req.SetBasicAuth(username, password)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var newStack map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&newStack)
	if newStack == nil || newStack["id"] == nil {
		return "", fmt.Errorf("stack created but no ID returned")
	}
	return fmt.Sprintf("%.0f", newStack["id"].(float64)), nil
}

func findOrCreateDeckCardInStack(apiURL, username, password, boardID, stackID, title string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Search all stacks for existing card
	req, _ := http.NewRequest("GET", apiURL+"/boards/"+boardID+"/stacks", nil)
	req.SetBasicAuth(username, password)
	req.Header.Set("OCS-APIRequest", "true")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var stacks []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&stacks)

	for _, st := range stacks {
		if st["id"] == nil {
			continue
		}
		sid := fmt.Sprintf("%.0f", st["id"].(float64))

		req, _ := http.NewRequest("GET", apiURL+"/boards/"+boardID+"/stacks/"+sid+"/cards", nil)
		req.SetBasicAuth(username, password)
		req.Header.Set("OCS-APIRequest", "true")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		var cards []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&cards)
		resp.Body.Close()

		for _, c := range cards {
			if c["title"] == title && c["id"] != nil {
				cardID := fmt.Sprintf("%.0f", c["id"].(float64))
				// Move card if in wrong stack
				if sid != stackID {
					moveCardToStack(apiURL, username, password, boardID, cardID, stackID)
				}
				return cardID, nil
			}
		}
	}

	// Create new card in target stack
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]interface{}{"title": title})
	url := apiURL + "/boards/" + boardID + "/stacks/" + stackID + "/cards"
	req, _ = http.NewRequest("POST", url, &buf)
	req.SetBasicAuth(username, password)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var newCard map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &newCard)
	if newCard == nil || newCard["id"] == nil {
		return "", fmt.Errorf("card created but no ID returned")
	}
	return fmt.Sprintf("%.0f", newCard["id"].(float64)), nil
}

func moveCardToStack(apiURL, username, password, boardID, cardID, stackID string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]interface{}{"stackId": stackID, "order": 999})

	req, _ := http.NewRequest("PUT", apiURL+"/boards/"+boardID+"/cards/"+cardID, &buf)
	req.SetBasicAuth(username, password)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func buildProgressDescription(course Course, store *CoachStore) string {
	progress := 0
	if course.TotalUnits > 0 {
		progress = (course.Completed * 100) / course.TotalUnits
	}

	emoji := "📚"
	if progress >= 100 {
		emoji = "✅"
	} else if progress > 0 {
		emoji = "🔥"
	}

	desc := fmt.Sprintf("%s **%s**\n\n", emoji, course.Name)
	desc += fmt.Sprintf("**Progress:** %d/%d units (%d%%)\n", course.Completed, course.TotalUnits, progress)
	desc += fmt.Sprintf("**Type:** %s\n\n", course.Type)

	// Weekly progress chart (last 4 weeks)
	desc += "📊 **Weekly Progress:**\n"
	weeks := []int{3, 5, 4, 2} // Mock data - replace with actual from store
	for i, w := range weeks {
		bar := strings.Repeat("█", w) + strings.Repeat("░", 7-w)
		desc += fmt.Sprintf("Week %d: %s %d units\n", i+1, bar, w)
	}

	// Monthly summary
	desc += "\n📈 **Monthly Summary:**\n"
	desc += fmt.Sprintf("This month: %d units\n", course.Completed)

	// Velocity
	if course.Pace7Day > 0 {
		desc += fmt.Sprintf("\n⚡ **Velocity:** %.1f units/week\n", course.Pace7Day*7)
	}

	return desc
}

func buildCourseLabels(course Course) []string {
	labels := []string{string(course.Type)}

	// Add keyword-based labels
	name := strings.ToLower(course.Name)
	keywords := map[string]string{
		"deep learning":    "AI",
		"machine learning": "ML",
		"python":           "Python",
		"javascript":       "JavaScript",
		"react":            "React",
		"ielts":            "IELTS",
		"english":          "Language",
		"math":             "Mathematics",
		"algorithm":        "Algorithms",
	}

	for keyword, label := range keywords {
		if strings.Contains(name, keyword) {
			labels = append(labels, label)
		}
	}

	return labels
}

func updateDeckCardWithLabels(apiURL, username, password, boardID, stackID, cardID, title, description string, labels []string) error {
	client := &http.Client{Timeout: 10 * time.Second}

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]interface{}{
		"title":       title,
		"description": description,
		"type":        "plain",
		"order":       999,
		"owner":       username,
	})
	req, _ := http.NewRequest("PUT", apiURL+"/boards/"+boardID+"/stacks/"+stackID+"/cards/"+cardID, &buf)
	req.SetBasicAuth(username, password)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
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
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Content-Type", "application/json")
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
