package coach

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sipeed/picoclaw/pkg/tools"
	_ "modernc.org/sqlite"
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
}

func NewSkill() *CoachSkill {
	return &CoachSkill{}
}

func (s *CoachSkill) Name() string {
	return "coach"
}

func (s *CoachSkill) Description() string {
	return `Momentum (Learning Coach) - Tracks study habits (IELTS, Exercise) via Nextcloud CalDAV, generates practice materials via WebDAV, and sends nudges via Telegram.

Commands:
- check_habits: Connects to Nextcloud CalDAV to check if daily VTODOs are checked off, then updates local SQLite streaks.
- generate_practice: Pulls random IELTS practice materials from Nextcloud WebDAV to provide an active study prompt.
- update_deck: Moves Kanban cards on Nextcloud Deck (e.g., To Do -> Done).
- nudge_telegram: Sends a personalized, energetic encouragement message directly to Jony's phone.`
}

func (s *CoachSkill) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command to execute",
				"enum":        []string{"check_habits", "generate_practice", "update_deck", "nudge_telegram"},
			},
			"practice_type": map[string]interface{}{
				"type":        "string",
				"description": "Type of IELTS material to pull (only for generate_practice)",
				"enum":        []string{"speaking_part_2", "speaking_part_3", "reading"},
			},
			"card_id": map[string]interface{}{
				"type":        "string",
				"description": "Deck card ID to move (only for update_deck)",
			},
			"column_id": map[string]interface{}{
				"type":        "string",
				"description": "Deck target column ID (only for update_deck)",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Text to send via Telegram (only for nudge_telegram)",
			},
		},
		"required": []string{"command"},
	}
}

func (s *CoachSkill) SetWorkspace(ws string) {
	s.workspace = ws
	s.initDB() // Init SQLite DB when workspace is set
	s.initWorkspace()
}

func (s *CoachSkill) initWorkspace() {
	if s.workspace == "" {
		return
	}
	identityPath := filepath.Join(s.workspace, "IDENTITY.md")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		identityContent := `# Learning Coach - Identity

- **Name:** Momentum
- **Creature:** Energetic coach with a whistle and stopwatch 🏃
- **Vibe:** "You got this!", celebrates wins, gentle with setbacks
- **Emoji:** 📚
- **Catchphrase:** "Streak alive! 🔥"
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

func (s *CoachSkill) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	command, _ := args["command"].(string)

	switch command {
	case "check_habits":
		return s.executeCheckHabits(ctx, args)
	case "generate_practice":
		return s.executeGeneratePractice(ctx, args)
	case "update_deck":
		return s.executeUpdateDeck(ctx, args)
	case "nudge_telegram":
		return s.executeNudgeTelegram(ctx, args)
	default:
		return tools.ErrorResult(fmt.Sprintf("Unknown command: %s", command))
	}
}

// ----------------------------------------------------------------------------
// SQLite Database Initiative
// ----------------------------------------------------------------------------

func (s *CoachSkill) initDB() {
	if s.workspace == "" {
		return
	}
	memDir := filepath.Join(s.workspace, "memory")
	os.MkdirAll(memDir, 0755)

	dbPath := filepath.Join(memDir, "momentum.db")
	db, err := sql.Open("sqlite", dbPath)
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

// ----------------------------------------------------------------------------
// Config Loading
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
