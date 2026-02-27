package coach

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/jony/son-of-anthon/pkg/skills/caldav"
	"github.com/sipeed/picoclaw/pkg/tools"
)

func buildFilesURL(cfg CoachConfig) string {
	// Appends the IELTS materials subdirectory onto the WebDAV base URL
	return caldav.BuildFilesURL(cfg.Host) + "IELTS_Materials/"
}

func buildDeckURL(cfg CoachConfig) string {
	return caldav.BuildDeckURL(cfg.Host)
}

// executeGeneratePractice pulls a random file from WebDAV
func (s *CoachSkill) executeGeneratePractice(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	cfg := loadCoachConfig()
	if cfg.Host == "" {
		return tools.ErrorResult("coach.host not configured in config.json")
	}

	filesURL := buildFilesURL(cfg)
	req, err := http.NewRequest("PROPFIND", filesURL, strings.NewReader(`<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`))
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to create WebDAV request: %v", err))
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
		return tools.ErrorResult(fmt.Sprintf("WebDAV PROPFIND failed: %v", err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Poor-man's XML parse for hrefs
	var files []string
	chunks := strings.Split(string(body), "<")
	basePath := ""

	for _, chunk := range chunks {
		lower := strings.ToLower(chunk)
		if strings.HasPrefix(lower, "d:href>") || strings.HasPrefix(lower, "href>") {
			parts := strings.SplitN(chunk, ">", 2)
			if len(parts) == 2 {
				href := strings.TrimSpace(parts[1])
				if basePath == "" {
					basePath = href // First one is the directory itself
				} else if href != basePath && !strings.HasSuffix(href, "/") {
					files = append(files, href)
				}
			}
		}
	}

	if len(files) == 0 {
		return tools.ErrorResult("The IELTS_Materials directory is empty. Please upload some PDFs, text files, or images to this folder in Nextcloud.")
	}

	// Pick random
	rand.Seed(time.Now().UnixNano())
	chosen := files[rand.Intn(len(files))]

	// Reconstruct full URL for Telegram
	filesURL = buildFilesURL(cfg)
	idx := strings.Index(filesURL, "/remote.php")
	fullURL := chosen
	if idx > 0 && !strings.HasPrefix(chosen, "http") {
		fullURL = filesURL[:idx] + chosen
	}

	result := fmt.Sprintf("Found practice material: %s\n\nPrompt the user to review this file.", fullURL)
	return &tools.ToolResult{ForLLM: result, ForUser: result}
}

func (s *CoachSkill) executeUpdateDeck(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	cfg := loadCoachConfig()
	cardID, _ := args["card_id"].(string)
	colID, _ := args["column_id"].(string)

	if cfg.Host == "" || cardID == "" || colID == "" {
		return tools.ErrorResult("coach.host, card_id, or column_id missing")
	}

	deckURL := buildDeckURL(cfg)
	url := fmt.Sprintf("%s/cards/%s", strings.TrimRight(deckURL, "/"), cardID)
	payload := fmt.Sprintf(`{"stackId": %s}`, colID) // Deck API moves via stackId update

	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(payload))
	if err != nil {
		return tools.ErrorResult(err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OCS-APIRequest", "true")
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
		return tools.ErrorResult(fmt.Sprintf("Deck API error: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return tools.ErrorResult(fmt.Sprintf("Deck returned %d: %s", resp.StatusCode, string(body)))
	}

	msg := fmt.Sprintf("Card %s moved to column %s successfully.", cardID, colID)
	return &tools.ToolResult{ForLLM: msg, ForUser: msg}
}

// executeNudgeTelegram sends a message to the unified Telegram chat
func (s *CoachSkill) executeNudgeTelegram(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	tgCfg := loadTelegramConfig()
	msg, _ := args["message"].(string)

	if tgCfg.BotToken == "" || tgCfg.ChatID == "" || msg == "" {
		return tools.ErrorResult("Telegram token, chat ID, or message missing")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", tgCfg.BotToken)

	payloadMap := map[string]interface{}{
		"chat_id":    tgCfg.ChatID,
		"text":       msg,
		"parse_mode": "Markdown",
	}
	payloadBytes, _ := json.Marshal(payloadMap)

	timeout := 10 * time.Second
	if tgCfg.Timeout > 0 {
		timeout = time.Duration(tgCfg.Timeout) * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to send Telegram message: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return tools.ErrorResult(fmt.Sprintf("Telegram API returned %d: %s", resp.StatusCode, string(body)))
	}

	result := "Telegram nudge sent successfully 🚀"
	return &tools.ToolResult{ForLLM: result, ForUser: result}
}
