package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jony/son-of-anthon/pkg/skills/architect"
)

func main() {
	s := architect.NewSkill()
	home, _ := os.UserHomeDir()
	workspace := fmt.Sprintf("%s/.picoclaw/workspace/architect", home)
	s.SetWorkspace(workspace)

	// Step 1: Create a recurring VTODO
	fmt.Println("=== Step 1: Creating 'Medicine Order' (recurring VTODO) ===")
	res := s.Execute(context.Background(), map[string]interface{}{
		"command":       "create_task",
		"title":         "Medicine Order",
		"task_type":     "recurring",
		"interval_days": float64(30),
		"target_date":   "2026-02-21",
	})
	if res.IsError {
		fmt.Printf("❌ %s\n", res.ForLLM)
	} else {
		fmt.Printf("✅ %s\n", res.ForUser)
	}

	// Step 2: Create a one-time VEVENT
	fmt.Println("\n=== Step 2: Creating 'Passport Renewal' (one-time VEVENT) ===")
	res2 := s.Execute(context.Background(), map[string]interface{}{
		"command":     "create_task",
		"title":       "Passport Renewal",
		"task_type":   "onetime",
		"target_date": "2026-02-24",
	})
	if res2.IsError {
		fmt.Printf("❌ %s\n", res2.ForLLM)
	} else {
		fmt.Printf("✅ %s\n", res2.ForUser)
	}

	// Step 3: Sync and read back
	fmt.Println("\n=== Step 3: Syncing deadlines dashboard ===")
	res3 := s.Execute(context.Background(), map[string]interface{}{"command": "sync_deadlines"})
	if res3.IsError {
		fmt.Printf("❌ sync_deadlines: %s\n", res3.ForLLM)
	}

	memFile := fmt.Sprintf("%s/memory/deadlines-today.md", workspace)
	data, _ := os.ReadFile(memFile)
	fmt.Println("\n--- TOKEN-OPTIMIZED DASHBOARD ---")
	fmt.Println(string(data))

	// Step 4: Debug — show raw fields from all .ics
	debugDumpTasks()
}

func debugDumpTasks() {
	home, _ := os.UserHomeDir()
	cfgPath := fmt.Sprintf("%s/.picoclaw/config.json", home)
	data, _ := os.ReadFile(cfgPath)
	cfg := parseConfig(data)

	fmt.Println("--- DEBUG: RAW TASK FIELDS FROM NEXTCLOUD TASKS CALENDAR ---")
	req, _ := http.NewRequest("PROPFIND", cfg.tasksURL,
		strings.NewReader(`<?xml version="1.0"?><propfind xmlns="DAV:"><prop><getetag/></prop></propfind>`))
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	req.SetBasicAuth(cfg.username, cfg.password)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("PROPFIND failed: %v\n", err)
		return
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
	fmt.Printf("Found %d .ics files\n\n", len(hrefs))

	idx := strings.Index(cfg.tasksURL, "/remote.php")
	baseURL := ""
	if idx > 0 {
		baseURL = cfg.tasksURL[:idx]
	}

	for i, href := range hrefs {
		fullURL := baseURL + href
		req2, _ := http.NewRequest(http.MethodGet, fullURL, nil)
		req2.SetBasicAuth(cfg.username, cfg.password)
		resp2, err := client.Do(req2)
		if err != nil {
			fmt.Printf("[%d] Error: %v\n", i, err)
			continue
		}
		icsBody, _ := io.ReadAll(resp2.Body)
		resp2.Body.Close()

		raw := strings.ReplaceAll(string(icsBody), "\r\n", "\n")
		raw = strings.ReplaceAll(raw, "\n ", "")

		fmt.Printf("--- Task %d ---\n", i+1)
		for _, line := range strings.Split(raw, "\n") {
			for _, key := range []string{"SUMMARY", "STATUS", "DUE", "DTSTART", "RRULE", "PERCENT-COMPLETE", "COMPLETED"} {
				if strings.HasPrefix(strings.ToUpper(line), key) {
					fmt.Printf("  %s\n", line)
				}
			}
		}
		fmt.Println()
	}
}

type simpleConfig struct{ tasksURL, username, password string }

func parseConfig(data []byte) simpleConfig {
	s := string(data)
	return simpleConfig{
		tasksURL: extractJSON(s, "tasks_url"),
		username: extractJSON(s, "username"),
		password: extractJSON(s, "password"),
	}
}

func extractJSON(s, key string) string {
	search := `"` + key + `": "`
	idx := strings.Index(s, search)
	if idx < 0 {
		return ""
	}
	start := idx + len(search)
	end := strings.Index(s[start:], `"`)
	if end < 0 {
		return ""
	}
	return s[start : start+end]
}
