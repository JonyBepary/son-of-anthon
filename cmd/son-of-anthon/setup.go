package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/charmbracelet/huh"
)

// setupCmd guides the user through interactively modifying their config.json
func setupCmd() {
	fmt.Printf("%s Starting Son of Anthon Setup Wizard...\n\n", logo)

	home, _ := os.UserHomeDir()
	configPath := os.Getenv("PERSONAL_OS_CONFIG")
	if configPath == "" {
		configPath = filepath.Join(home, ".picoclaw", "config.json")
	}

	// Ensure config directory exists
	os.MkdirAll(filepath.Dir(configPath), 0755)

	// Also ensure workspace directory exists
	wsDir := filepath.Join(home, ".picoclaw", "workspace")
	os.MkdirAll(wsDir, 0755)

	rawCfg := make(map[string]interface{})
	data, err := os.ReadFile(configPath)
	if err == nil {
		json.Unmarshal(data, &rawCfg)
	} else {
		fmt.Printf("Warning: Failed to load existing configuration (%v). Starting with a blank configuration.\n", err)
	}

	// Helper function to safely navigate and create maps for arbitrary JSON nestings
	ensureMap := func(m map[string]interface{}, key string) map[string]interface{} {
		if m[key] == nil {
			m[key] = make(map[string]interface{})
		}
		if val, ok := m[key].(map[string]interface{}); ok {
			return val
		}
		newMap := make(map[string]interface{})
		m[key] = newMap
		return newMap
	}

	agents := ensureMap(rawCfg, "agents")
	defaults := ensureMap(agents, "defaults")
	providers := ensureMap(rawCfg, "providers")
	tools := ensureMap(rawCfg, "tools")
	telegramCfg := ensureMap(tools, "telegram")
	channels := ensureMap(rawCfg, "channels")
	telegramChannel := ensureMap(channels, "telegram")
	nextcloudCfg := ensureMap(tools, "nextcloud")
	webCfg := ensureMap(tools, "web")
	braveCfg := ensureMap(webCfg, "brave")
	heartbeatCfg := ensureMap(rawCfg, "heartbeat")

	// Helper to extract strings safely
	getString := func(m map[string]interface{}, key, def string) string {
		if v, ok := m[key].(string); ok {
			return v
		}
		return def
	}

	llmProvider := getString(defaults, "provider", "nvidia")
	llmModel := getString(defaults, "model", "qwen/qwen3.5-397b-a17b")
	// Get existing api_base if set (for display purposes)
	var customAPIBase string
	if pMap, ok := providers[llmProvider].(map[string]interface{}); ok {
		customAPIBase = getString(pMap, "api_base", "")
	}

	var maxTokensStr string
	if v, ok := defaults["max_tokens"].(float64); ok {
		maxTokensStr = strconv.Itoa(int(v))
	} else if v, ok := defaults["max_tokens"].(int); ok {
		maxTokensStr = strconv.Itoa(v)
	} else {
		maxTokensStr = "8192"
	}

	var temperatureStr string
	if v, ok := defaults["temperature"].(float64); ok {
		temperatureStr = strconv.FormatFloat(v, 'f', -1, 64)
	} else {
		temperatureStr = "0.7"
	}

	var maxToolIterStr string
	if v, ok := defaults["max_tool_iterations"].(float64); ok {
		maxToolIterStr = strconv.Itoa(int(v))
	} else if v, ok := defaults["max_tool_iterations"].(int); ok {
		maxToolIterStr = strconv.Itoa(v)
	} else {
		maxToolIterStr = "20"
	}

	var providerKey string
	if pMap, ok := providers[llmProvider].(map[string]interface{}); ok {
		providerKey = getString(pMap, "api_key", "")
	}

	tgToken := getString(telegramCfg, "bot_token", "")
	tgChat := getString(telegramCfg, "chat_id", "")

	ncHost := getString(nextcloudCfg, "host", "")
	ncCal := getString(nextcloudCfg, "calendar_url", "")
	ncTask := getString(nextcloudCfg, "tasks_url", "")
	ncFile := getString(nextcloudCfg, "files_url", "")
	ncDeck := getString(nextcloudCfg, "deck_url", "")
	ncUser := getString(nextcloudCfg, "username", "")
	ncPass := getString(nextcloudCfg, "password", "")
	braveKey := getString(braveCfg, "api_key", "")

	isAdvancedNextcloud := (ncHost == "" && (ncCal != "" || ncTask != "" || ncFile != "" || ncDeck != ""))

	// Extract heartbeat numeric interval safely
	var hbIntervalStr string
	if v, ok := heartbeatCfg["interval"].(float64); ok {
		hbIntervalStr = strconv.FormatFloat(v, 'f', -1, 64)
	} else if v, ok := heartbeatCfg["interval"].(int); ok {
		hbIntervalStr = strconv.Itoa(v)
	} else {
		hbIntervalStr = "30" // Default
	}

	llmConfigLevel := "Basic (Default)"

	// Create the form
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("1. What LLM Provider do you want to use?").
				Options(
					huh.NewOption("Qwen via NVIDIA NIM (Recommended)", "nvidia"),
					huh.NewOption("OpenRouter (Universal)", "openrouter"), huh.NewOption("OpenAI", "openai"),
					huh.NewOption("Anthropic (Claude)", "anthropic"),
					huh.NewOption("Ollama (Local)", "ollama"),
				).
				Value(&llmProvider),

			huh.NewInput().
				Title("API Key").
				Description("Enter the API token for your provider.").
				EchoMode(huh.EchoModePassword).
				Value(&providerKey),

			huh.NewInput().
				Title("LLM Model Name").
				Description("Examples: 'qwen/qwen3.5-397b-a17b' or 'nvidia/llama-3.1-nemotron-70b-instruct'").
				Value(&llmModel),

			huh.NewInput().
				Title("Custom API Base URL (Optional)").
				Description("For NVIDIA NIM: https://integrate.api.nvidia.com/v1").
				Value(&customAPIBase),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("LLM Configuration Level").
				Options(
					huh.NewOption("Basic (Default)", "Basic (Default)"),
					huh.NewOption("Advanced Options", "Advanced"),
				).
				Value(&llmConfigLevel),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Max Tokens").
				Description("Maximum output tokens generated per sequence (e.g. 8192).").
				Value(&maxTokensStr),

			huh.NewInput().
				Title("Temperature").
				Description("Creativity/randomness coefficient from 0.0 to 1.0 (e.g. 0.7).").
				Value(&temperatureStr),

			huh.NewInput().
				Title("Max Tool Iterations").
				Description("Number of maximum sequential tool calls before forcing the agent to exit (e.g. 20).").
				Value(&maxToolIterStr),
		).WithHideFunc(func() bool {
			return llmConfigLevel != "Advanced"
		}),
		huh.NewGroup(
			huh.NewInput().
				Title("2. Telegram Bot Token").
				Description("Ask @BotFather on Telegram for a new bot token. Leave blank to disable channels.").
				EchoMode(huh.EchoModePassword).
				Value(&tgToken),

			huh.NewInput().
				Title("Telegram Admin Chat ID").
				Description("Your numeric UID so no random strangers can talk to the bot.").
				Value(&tgChat),

			huh.NewInput().
				Title("3. Brave Search API Key").
				Description("Leave blank if you don't use Brave Web Search.").
				Value(&braveKey),
		).Title("Channels & Search Configuration"),
		huh.NewGroup(
			huh.NewInput().Title("3. Wakeup Heartbeat Interval (Minutes)").Value(&hbIntervalStr).Description("How frequently the agent auto-wakes (e.g. 30). Set to 0 to disable."),
		).Title("Daemon Settings"),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Use Advanced Configuration for Nextcloud?").
				Description("Select NO to just provide a single Nextcloud Host URL. Select YES to provide separate URLs for Calendar, Tasks, Files, and Deck.").
				Value(&isAdvancedNextcloud),
		).Title("Self-Hosted Ecosystem"),
		huh.NewGroup(
			huh.NewInput().Title("4. Nextcloud Host URL").Value(&ncHost).Description("Ex: https://ivo.lv.tab.digital"),
		).WithHideFunc(func() bool {
			return isAdvancedNextcloud
		}),
		huh.NewGroup(
			huh.NewInput().Title("4. Calendar (VEVENT) URL").Value(&ncCal),
			huh.NewInput().Title("Tasks (VTODO) URL").Value(&ncTask),
			huh.NewInput().Title("WebDAV Files URL").Value(&ncFile),
			huh.NewInput().Title("Deck API URL").Value(&ncDeck),
		).WithHideFunc(func() bool {
			return !isAdvancedNextcloud
		}),
		huh.NewGroup(
			huh.NewInput().Title("Nextcloud Username").Value(&ncUser).Description("Ex: email@example.com"),
			huh.NewInput().Title("Nextcloud App Password").EchoMode(huh.EchoModePassword).Value(&ncPass),
		),
	)

	err = form.Run()
	if err != nil {
		log.Fatalf("Form aborted: %v", err)
	}

	// Apply mutated values back to the map
	defaults["provider"] = llmProvider
	defaults["model"] = llmModel

	if mt, err := strconv.Atoi(maxTokensStr); err == nil {
		defaults["max_tokens"] = mt
	} else {
		defaults["max_tokens"] = 8192
	}

	if temp, err := strconv.ParseFloat(temperatureStr, 64); err == nil {
		defaults["temperature"] = temp
	} else {
		defaults["temperature"] = 0.7
	}

	if mti, err := strconv.Atoi(maxToolIterStr); err == nil {
		defaults["max_tool_iterations"] = mti
	} else {
		defaults["max_tool_iterations"] = 20
	}

	defaults["restrict_to_workspace"] = true

	if providerKey != "" {
		pMap := ensureMap(providers, llmProvider)
		pMap["api_key"] = providerKey
		if customAPIBase != "" {
			pMap["api_base"] = customAPIBase
		}

		// Also set up model_list for proper provider routing
		modelList := []map[string]interface{}{
			{
				"provider":   llmProvider,
				"model":      llmModel,
				"model_name": llmModel,
				"api_key":    providerKey,
			},
		}
		if customAPIBase != "" {
			modelList[0]["api_base"] = customAPIBase
			// Also save to providers for backwards compatibility
			pMap["api_base"] = customAPIBase
		}
		rawCfg["model_list"] = modelList
	}

	if hbInt, err := strconv.Atoi(hbIntervalStr); err == nil {
		heartbeatCfg["interval"] = hbInt
		heartbeatCfg["enabled"] = hbInt > 0
	}

	if braveKey != "" {
		braveCfg["enabled"] = true
		braveCfg["api_key"] = braveKey
		braveCfg["max_results"] = 5
	} else {
		braveCfg["enabled"] = false
		delete(braveCfg, "api_key")
	}

	// tools.telegram — used by Son of Anthon's skill for sending nudges
	telegramCfg["bot_token"] = tgToken
	telegramCfg["chat_id"] = tgChat

	// channels.telegram — used by picoclaw framework to start the polling daemon
	telegramChannel["enabled"] = tgToken != ""
	telegramChannel["token"] = tgToken
	if tgChat != "" {
		telegramChannel["allow_from"] = []string{tgChat}
	} else {
		delete(telegramChannel, "allow_from")
	}

	if isAdvancedNextcloud {
		nextcloudCfg["calendar_url"] = ncCal
		nextcloudCfg["tasks_url"] = ncTask
		nextcloudCfg["files_url"] = ncFile
		nextcloudCfg["deck_url"] = ncDeck
		delete(nextcloudCfg, "host")
	} else {
		nextcloudCfg["host"] = ncHost
		delete(nextcloudCfg, "calendar_url")
		delete(nextcloudCfg, "tasks_url")
		delete(nextcloudCfg, "files_url")
		delete(nextcloudCfg, "deck_url")
	}

	nextcloudCfg["username"] = ncUser
	nextcloudCfg["password"] = ncPass

	// Revert them cleanly if empty
	cleanEmptyStrings := func(m map[string]interface{}) {
		for k, v := range m {
			if str, ok := v.(string); ok && str == "" {
				delete(m, k)
			}
		}
	}
	cleanEmptyStrings(telegramCfg)
	cleanEmptyStrings(nextcloudCfg)

	// Save back to disk
	file, err := os.Create(configPath)
	if err != nil {
		log.Fatalf("Failed to open %s for writing: %v", configPath, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(rawCfg); err != nil {
		log.Fatalf("Failed to serialize config.json: %v", err)
	}

	fmt.Printf("\n✅ Setup complete! Configuration cleanly saved to %s\n", configPath)
	fmt.Printf("Run `./son-of-anthon gateway` to spin up your bot!\n")
}
