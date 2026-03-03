package coach

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jony/son-of-anthon/pkg/sqlite"
	"github.com/sipeed/picoclaw/pkg/tools"
)

type CoachConfig struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
	Timeout  int    `json:"timeout_seconds"`
}

type TelegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
	Timeout  int    `json:"timeout_seconds"`
}

type CoachSkill struct {
	workspace string
	db        *sql.DB
	store     *CoachStore
}

func NewSkill() *CoachSkill {
	return &CoachSkill{}
}

func (s *CoachSkill) Name() string {
	return "coach"
}

func (s *CoachSkill) Description() string {
	return `Learning Coach - Tracks courses, books, and videos with progress and pace tracking.

Commands:
- add_course: Add a new course/book/video with chapter list
- my_courses: List all active courses with progress
- progress: Show detailed progress for a specific course
- weekly: Show this week's learning stats
- log_progress: Mark chapters/videos as completed
- estimate_finish: Calculate ETA based on current pace
- sync_deck: Sync courses to Nextcloud Deck
- sync_tasks: Sync units to Nextcloud Tasks (CalDAV)
- sync_calendar: Sync study sessions to Nextcloud Calendar

Examples:
- "add_course Deep Learning book chapters 1-15"
- "what am i studying"
- "finished chapter 5"
- "how far in deep learning"
- "show weekly progress"
- "sync to nextcloud deck"
`
}

func (s *CoachSkill) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command to execute",
				"enum":        []string{"add_course", "my_courses", "progress", "weekly", "log_progress", "estimate_finish", "sync_deck", "sync_tasks", "sync_calendar"},
			},
			"course_name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the course (for add_course, progress, estimate_finish)",
			},
			"course_type": map[string]interface{}{
				"type":        "string",
				"description": "Type: book, video, or custom",
				"enum":        []string{"book", "video", "custom"},
			},
			"units": map[string]interface{}{
				"type":        "string",
				"description": "List of units (chapters/videos) to add, comma or newline separated",
			},
			"total_units": map[string]interface{}{
				"type":        "number",
				"description": "Total number of units if not providing full list",
			},
			"completed_units": map[string]interface{}{
				"type":        "string",
				"description": "Units to mark as completed (e.g., '5' or '1-5')",
			},
		},
		"required": []string{"command"},
	}
}

func (s *CoachSkill) SetWorkspace(ws string) {
	s.workspace = ws
	s.initStore()
	s.initWorkspace()
}

func (s *CoachSkill) initStore() {
	if s.workspace == "" {
		return
	}
	memDir := filepath.Join(s.workspace, "memory")
	os.MkdirAll(memDir, 0755)

	// Initialize course tracking store
	store, err := NewCoachStore(memDir)
	if err != nil {
		fmt.Printf("[Coach] Error initializing store: %v\n", err)
		return
	}
	s.store = store

	// Initialize legacy streaks database for backwards compatibility
	dbPath := filepath.Join(memDir, "momentum.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		fmt.Printf("[Coach] Error opening SQLite database: %v\n", err)
		return
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS streaks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		category TEXT UNIQUE NOT NULL,
		current_streak INTEGER DEFAULT 0,
		last_completed_date TEXT
	);`

	if _, err := db.Exec(createTableSQL); err != nil {
		fmt.Printf("[Coach] Error creating streaks table: %v\n", err)
		db.Close()
		return
	}

	s.db = db
}

func (s *CoachSkill) initWorkspace() {
	if s.workspace == "" {
		return
	}
	identityPath := filepath.Join(s.workspace, "IDENTITY.md")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		identityContent := `# Learning Coach - Identity

- **Name:** Coach
- **Creative Name:** Momentum
- **Vibe:** "You got this!", celebrates wins, tracks pace
- **Emoji:** 📚
- **Catchphrase:** "Keep the momentum! 🔥"
`
		os.WriteFile(identityPath, []byte(identityContent), 0644)
	}
}

func (s *CoachSkill) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	command, _ := args["command"].(string)

	switch command {
	case "add_course":
		return s.executeAddCourse(ctx, args)
	case "my_courses":
		return s.executeMyCourses(ctx, args)
	case "progress":
		return s.executeProgress(ctx, args)
	case "weekly":
		return s.executeWeekly(ctx, args)
	case "log_progress":
		return s.executeLogProgress(ctx, args)
	case "estimate_finish":
		return s.executeEstimateFinish(ctx, args)
	case "sync_deck":
		return s.executeSyncDeck(ctx, args)
	case "sync_tasks":
		return s.executeSyncTasks(ctx, args)
	case "sync_calendar":
		return s.executeSyncCalendar(ctx, args)
	default:
		return tools.ErrorResult(fmt.Sprintf("Unknown command: %s", command))
	}
}

// ----------------------------------------------------------------------------
// Course Tracking Commands
// ----------------------------------------------------------------------------

func (s *CoachSkill) executeAddCourse(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	if s.store == nil {
		return tools.ErrorResult("Coach store not initialized")
	}

	courseName, _ := args["course_name"].(string)
	if courseName == "" {
		return tools.ErrorResult("course_name required")
	}

	courseType, _ := args["course_type"].(string)
	if courseType == "" {
		courseType = "book"
	}

	unitsStr, _ := args["units"].(string)
	totalUnits := 0
	if total, ok := args["total_units"].(float64); ok {
		totalUnits = int(total)
	}

	// Parse units
	var units []string
	if unitsStr != "" {
		units = strings.Split(unitsStr, ",")
		for i := range units {
			units[i] = strings.TrimSpace(units[i])
		}
		if totalUnits == 0 {
			totalUnits = len(units)
		}
	}

	if totalUnits == 0 {
		return tools.ErrorResult("Provide either units list or total_units")
	}

	// Create course
	now := time.Now()
	course := &Course{
		ID:         fmt.Sprintf("course-%d", now.Unix()),
		Name:       courseName,
		Type:       CourseType(courseType),
		Source:     "manual",
		TotalUnits: totalUnits,
		Completed:  0,
		Pace7Day:   0,
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.store.CreateCourse(course); err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to create course: %v", err))
	}

	// Add units if provided
	for i, title := range units {
		unit := &Unit{
			ID:       fmt.Sprintf("unit-%d-%d", now.Unix(), i),
			CourseID: course.ID,
			Index:    i + 1,
			Title:    title,
			Duration: 30, // default 30 min
			Status:   "pending",
		}
		s.store.AddUnit(unit)
	}

	return &tools.ToolResult{
		ForUser: fmt.Sprintf("✅ Added course: %s (%s, %d units)", courseName, courseType, totalUnits),
		ForLLM:  fmt.Sprintf("Course %s created with %d units", courseName, totalUnits),
	}
}

func (s *CoachSkill) executeMyCourses(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	if s.store == nil {
		return tools.ErrorResult("Coach store not initialized")
	}

	result, err := s.store.ActiveCoursesJSON()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Error: %v", err))
	}

	return &tools.ToolResult{
		ForUser: "📚 Your Active Courses:\n\n" + result,
		ForLLM:  result,
	}
}

func (s *CoachSkill) executeProgress(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	if s.store == nil {
		return tools.ErrorResult("Coach store not initialized")
	}

	courseName, _ := args["course_name"].(string)
	if courseName == "" {
		return tools.ErrorResult("course_name required")
	}

	result, err := s.store.CourseProgressJSON(courseName)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Error: %v", err))
	}

	return &tools.ToolResult{
		ForUser: "📊 Progress:\n\n" + result,
		ForLLM:  result,
	}
}

func (s *CoachSkill) executeWeekly(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	if s.store == nil {
		return tools.ErrorResult("Coach store not initialized")
	}

	result, err := s.store.WeeklyStatsJSON()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Error: %v", err))
	}

	return &tools.ToolResult{
		ForUser: "📈 This Week's Stats:\n\n" + result,
		ForLLM:  result,
	}
}

func (s *CoachSkill) executeLogProgress(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	if s.store == nil {
		return tools.ErrorResult("Coach store not initialized")
	}

	courseName, _ := args["course_name"].(string)
	completedUnits, _ := args["completed_units"].(string)

	if courseName == "" || completedUnits == "" {
		return tools.ErrorResult("course_name and completed_units required")
	}

	// Find course
	courses, err := s.store.GetActiveCourses()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Error: %v", err))
	}

	var target *Course
	for _, c := range courses {
		if strings.Contains(strings.ToLower(c.Name), strings.ToLower(courseName)) {
			target = c
			break
		}
	}

	if target == nil {
		return tools.ErrorResult(fmt.Sprintf("Course not found: %s", courseName))
	}

	// Parse completed units (single number or range like "1-5")
	var unitsDone int
	if strings.Contains(completedUnits, "-") {
		parts := strings.Split(completedUnits, "-")
		if len(parts) == 2 {
			var start, end int
			fmt.Sscanf(parts[0], "%d", &start)
			fmt.Sscanf(parts[1], "%d", &end)
			unitsDone = end - start + 1
		}
	} else {
		fmt.Sscanf(completedUnits, "%d", &unitsDone)
	}

	// Get units and complete them
	units, err := s.store.GetCourseUnits(target.ID)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Error: %v", err))
	}

	completed := 0
	now := time.Now()
	for _, u := range units {
		if u.Status == "pending" && completed < unitsDone {
			s.store.db.Exec("UPDATE units SET status = 'completed', completed_at = ? WHERE id = ?", now.Unix(), u.ID)
			completed++
		}
	}

	// Log session
	session := &Session{
		ID:        fmt.Sprintf("session-%d", now.Unix()),
		CourseID:  target.ID,
		UnitsDone: completed,
		StartedAt: now,
		EndedAt:   now,
	}
	s.store.AddSession(session)

	// Update course completed count
	s.store.db.Exec("UPDATE courses SET completed = completed + ?, updated_at = ? WHERE id = ?", completed, now.Unix(), target.ID)

	return &tools.ToolResult{
		ForUser: fmt.Sprintf("✅ Logged %d completed units for %s", completed, target.Name),
		ForLLM:  fmt.Sprintf("Completed %d units", completed),
	}
}

func (s *CoachSkill) executeEstimateFinish(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	if s.store == nil {
		return tools.ErrorResult("Coach store not initialized")
	}

	courseName, _ := args["course_name"].(string)
	if courseName == "" {
		return tools.ErrorResult("course_name required")
	}

	progress, err := s.store.GetCourseProgress(courseName)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Error: %v", err))
	}

	daysLeft := progress["days_left"].(int)
	pace := progress["pace_7day"].(float64)

	var eta string
	if pace <= 0 {
		eta = "No pace data yet - log some progress first!"
	} else if daysLeft <= 0 {
		eta = "You're behind! Need to speed up."
	} else if daysLeft <= 7 {
		eta = fmt.Sprintf("~%d days (on track!)", daysLeft)
	} else {
		eta = fmt.Sprintf("~%d days", daysLeft)
	}

	return &tools.ToolResult{
		ForUser: fmt.Sprintf("📅 %s\nPace: %.1f units/day\nETA: %s", progress["course"], pace, eta),
		ForLLM:  fmt.Sprintf("ETA: %s", eta),
	}
}

// ----------------------------------------------------------------------------
// Legacy Nextcloud Commands (kept for backwards compatibility)
// ----------------------------------------------------------------------------

func loadCoachConfig() CoachConfig {
	var cfg struct {
		Tools struct {
			Nextcloud CoachConfig `json:"nextcloud"`
		} `json:"tools"`
	}
	home, _ := os.UserHomeDir()
	path := os.Getenv("PERSONAL_OS_CONFIG")
	if path == "" {
		path = filepath.Join(home, ".picoclaw", "config.json")
	}
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &cfg)
	}
	return cfg.Tools.Nextcloud
}

func loadTelegramConfig() TelegramConfig {
	var cfg struct {
		Tools struct {
			Telegram TelegramConfig `json:"telegram"`
		} `json:"tools"`
	}
	home, _ := os.UserHomeDir()
	path := os.Getenv("PERSONAL_OS_CONFIG")
	if path == "" {
		path = filepath.Join(home, ".picoclaw", "config.json")
	}
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &cfg)
	}
	return cfg.Tools.Telegram
}
