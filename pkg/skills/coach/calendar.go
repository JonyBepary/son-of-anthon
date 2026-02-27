package coach

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jony/son-of-anthon/pkg/skills/caldav"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// executeCheckHabits implements the CalDAV PROPFIND + GET check.
func (s *CoachSkill) executeCheckHabits(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	cfg := loadCoachConfig()
	if cfg.Host == "" {
		return tools.ErrorResult("coach.host not configured in config.json")
	}

	hrefs, err := listNextcloudTasks(cfg)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to list tasks: %v", err))
	}

	todayStr := time.Now().Format("20060102") // e.g. 20260221

	habitCompleted := map[string]bool{
		"IELTS":    false,
		"Exercise": false,
	}

	for _, href := range hrefs {
		fields, err := getTaskFromCalDAV(cfg, href)
		if err != nil {
			continue // skip errors
		}

		summary := strings.ToLower(fields["SUMMARY"])
		status := fields["STATUS"]
		pct := fields["PERCENT-COMPLETE"]
		completedTimestamp := fields["COMPLETED"]
		lastModified := fields["LAST-MODIFIED"]

		// Determine if it was completed today
		isCompleted := status == "COMPLETED" || pct == "100"
		completedToday := false

		if isCompleted {
			if completedTimestamp != "" && strings.HasPrefix(completedTimestamp, todayStr) {
				completedToday = true
			} else if lastModified != "" && strings.HasPrefix(lastModified, todayStr) {
				completedToday = true
			} else if completedTimestamp == "" && lastModified == "" {
				// Fallback if no timestamp found but it is completed
				completedToday = true
			}
		}

		if completedToday {
			if strings.Contains(summary, "ielts") {
				habitCompleted["IELTS"] = true
			}
			if strings.Contains(summary, "exercise") {
				habitCompleted["Exercise"] = true
			}
		}
	}

	// Now update streaks in SQLite
	out := s.updateStreaks(habitCompleted)
	return &tools.ToolResult{ForLLM: out, ForUser: out}
}

// updateStreaks logs the completions to SQLite and returns a status string.
func (s *CoachSkill) updateStreaks(completed map[string]bool) string {
	if s.db == nil {
		return "⚠️ SQLite DB not initialized."
	}

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	var sb strings.Builder
	sb.WriteString("Habit Check Results:\n")

	for category, didComplete := range completed {
		var currentStreak int
		var lastCompleted string

		// Get current streak
		err := s.db.QueryRow("SELECT current_streak, last_completed_date FROM streaks WHERE category = ?", category).Scan(&currentStreak, &lastCompleted)

		if err != nil {
			// Doesn't exist, insert
			if didComplete {
				s.db.Exec("INSERT INTO streaks (category, current_streak, last_completed_date) VALUES (?, 1, ?)", category, today)
				sb.WriteString(fmt.Sprintf("- %s: Started new streak! 🔥 (1 day)\n", category))
			} else {
				s.db.Exec("INSERT INTO streaks (category, current_streak, last_completed_date) VALUES (?, 0, '')", category)
				sb.WriteString(fmt.Sprintf("- %s: Not started yet.\n", category))
			}
			continue
		}

		if didComplete {
			if lastCompleted == today {
				sb.WriteString(fmt.Sprintf("- %s: Already logged today. Active streak: %d days 🔥\n", category, currentStreak))
			} else if lastCompleted == yesterday {
				currentStreak++
				s.db.Exec("UPDATE streaks SET current_streak = ?, last_completed_date = ? WHERE category = ?", currentStreak, today, category)
				sb.WriteString(fmt.Sprintf("- %s: Streak extended! Active streak: %d days 🔥\n", category, currentStreak))
			} else {
				// Broken streak, restarting
				s.db.Exec("UPDATE streaks SET current_streak = 1, last_completed_date = ? WHERE category = ?", today, category)
				sb.WriteString(fmt.Sprintf("- %s: Streak restarted! (1 day) 🌱\n", category))
			}
		} else {
			if lastCompleted == today {
				sb.WriteString(fmt.Sprintf("- %s: Completed for today! 🔥 (%d days)\n", category, currentStreak))
			} else if lastCompleted == yesterday {
				sb.WriteString(fmt.Sprintf("- %s: Pending for today. Don't lose your %d day streak!\n", category, currentStreak))
			} else {
				if currentStreak > 0 {
					// Streak broke
					s.db.Exec("UPDATE streaks SET current_streak = 0 WHERE category = ?", category)
				}
				sb.WriteString(fmt.Sprintf("- %s: No active streak. Ready to jump back in? 🌱\n", category))
			}
		}
	}

	return sb.String()
}

// ----------------------------------------------------------------------------
// CalDAV Helpers
// ----------------------------------------------------------------------------

func buildTasksURL(cfg CoachConfig) string {
	return caldav.BuildTasksURL(cfg.Host, cfg.Username)
}

func listNextcloudTasks(cfg CoachConfig) ([]string, error) {
	base := buildTasksURL(cfg)
	req, err := http.NewRequest("PROPFIND", base, strings.NewReader(`<?xml version="1.0"?><propfind xmlns="DAV:"><prop><getetag/></prop></propfind>`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	if cfg.Username != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}
	timeout := 10 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PROPFIND failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading PROPFIND response: %w", err)
	}

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

func getTaskFromCalDAV(cfg CoachConfig, href string) (map[string]string, error) {
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
	if cfg.Username != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}
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
		case "SUMMARY", "STATUS", "PERCENT-COMPLETE", "COMPLETED", "LAST-MODIFIED":
			// Unescape
			val = strings.ReplaceAll(val, "\\,", ",")
			val = strings.ReplaceAll(val, "\\;", ";")
			val = strings.ReplaceAll(val, "\\n", "\n")
			fields[key] = val
		}
	}
	return fields, nil
}
