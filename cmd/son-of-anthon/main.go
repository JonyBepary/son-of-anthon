package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/cron"
	"github.com/sipeed/picoclaw/pkg/devices"
	"github.com/sipeed/picoclaw/pkg/health"
	"github.com/sipeed/picoclaw/pkg/heartbeat"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/state"
	"github.com/sipeed/picoclaw/pkg/tools"
	"github.com/sipeed/picoclaw/pkg/voice"

	"github.com/jony/son-of-anthon/pkg/skills/architect"
	"github.com/jony/son-of-anthon/pkg/skills/atc"
	"github.com/jony/son-of-anthon/pkg/skills/chief"
	"github.com/jony/son-of-anthon/pkg/skills/coach"
	"github.com/jony/son-of-anthon/pkg/skills/monitor"
	"github.com/jony/son-of-anthon/pkg/skills/research"
	"github.com/jony/son-of-anthon/pkg/skills/subagent"
	"github.com/jony/son-of-anthon/workspaces"
)

const logo = "🎯"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "agent":
		agentCmd()
	case "gateway":
		gatewayCmd()
	case "setup":
		setupCmd()
	case "version", "--version", "-v":
		fmt.Printf("%s son-of-anthon v1.0.0\n", logo)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf("%s son-of-anthon - Multi-agent AI Assistant\n\n", logo)
	fmt.Println("Usage: son-of-anthon <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  agent     Interact with the main agent")
	fmt.Println("  gateway   Start the background daemon with Telegram/Cron/Heartbeat")
	fmt.Println("  setup     Run interactive UI to configure API keys and connections")
	fmt.Println("  version   Show version")
}

func loadConfig() (*config.Config, error) {
	home, _ := os.UserHomeDir()
	configPath := os.Getenv("PERSONAL_OS_CONFIG")
	if configPath == "" {
		configPath = filepath.Join(home, ".picoclaw", "config.json")
	}

	// Auto-initialize ~/.picoclaw from local ./config.json if missing
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(configPath), 0755)
		if data, err := os.ReadFile("config.json"); err == nil {
			os.WriteFile(configPath, data, 0644)
		}
	}

	// Auto-copy embedded workspaces to global if global doesn't exist
	wsDir := filepath.Join(home, ".picoclaw", "workspace")
	if _, err := os.Stat(wsDir); os.IsNotExist(err) {
		os.MkdirAll(wsDir, 0755)
		err := copyEmbedToDisk(wsDir)
		if err != nil {
			log.Printf("Failed to initialize workspace: %v\n", err)
		} else {
			log.Println("Initialized new workspace at:", wsDir)
		}
	}

	// Check if config exists, if not run interactive setup
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("No config found. Running setup wizard...")
		setupCmd()

		// After setup, verify config was created
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("setup incomplete: config.json not created")
		}
	}

	identityPath := filepath.Join(wsDir, "IDENTITY.md")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		identityContent := `You are son-of-anthon, a personal multi-agent AI assistant.

Available tools (call as needed, including multiple times in one session):
- architect: Life admin; CalDAV sync/create/delete tasks on Nextcloud. Commands: sync_deadlines, create_task, delete_task
- chief: Strategic commander; reads daily briefs, urgent deadlines, morning/evening summaries. Commands: morning_brief, evening_review, urgent_deadlines, status, delegate
- atc: Task management; reads/writes tasks.xml, daily priorities. Commands: analyze_tasks, read_calendar, update_task, roll_over_tasks, sync_calendar, push_task
- coach: Learning coach; IELTS prep, habit tracking, Nextcloud integration. Commands: check_habits, fetch_material, generate_practice, evening_review, update_deck, nudge_telegram
- monitor: News curation; Bangladesh + Tech RSS feeds. Commands: fetch, status, feeds
- research: Academic paper discovery from ArXiv and HuggingFace. Commands: fetch
- subagent: Spawn any of the above as a dedicated subagent with deeper context

IMPORTANT RENDERING RULES:
- For morning_brief, evening_review, fetch news, search papers: reproduce the full tool output verbatim. Do NOT summarize or wrap in <status> tags.
- For create/delete/sync actions: a short confirmation is fine.
- When the user asks for a multi-step task, call tools sequentially as needed.`
		os.WriteFile(identityPath, []byte(identityContent), 0644)
	}

	heartbeatPath := filepath.Join(wsDir, "HEARTBEAT.md")
	if _, err := os.Stat(heartbeatPath); os.IsNotExist(err) {
		heartbeatContent := `# Heartbeat Instruction
The Zero-Cost Go interceptor has woken you up because something is urgently pending.
Check the urgent_deadlines tool to review deadlines, or analyze_tasks for ATC tasks.
If there are items that are P0 or expiring soon which user needs to know about, notify them.
If nothing is urgent, just reply: HEARTBEAT_OK`
		os.WriteFile(heartbeatPath, []byte(heartbeatContent), 0644)
	}

	return config.LoadConfig(configPath)
}

// Helper function to copy embedded files to disk
func copyEmbedToDisk(dst string) error {
	return fs.WalkDir(workspaces.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." || path == "embed.go" {
			return nil
		}

		targetPath := filepath.Join(dst, path)
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		data, err := workspaces.FS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0644)
	})
}

func resolveWorkspacePath(relativePath string) string {
	home, _ := os.UserHomeDir()
	name := filepath.Base(relativePath)
	return filepath.Join(home, ".picoclaw", "workspace", name)
}

func agentCmd() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	provider, err := providers.CreateProvider(cfg)
	if err != nil {
		fmt.Printf("Error creating provider: %v\n", err)
		os.Exit(1)
	}

	workspace := cfg.WorkspacePath()
	if workspace == "" {
		home, _ := os.UserHomeDir()
		workspace = fmt.Sprintf("%s/.picoclaw/workspace", home)
	}

	// Resolve monitor workspace relative to project root
	monitorWorkspace := resolveWorkspacePath("workspaces/monitor")

	toolsRegistry := tools.NewToolRegistry()

	researchWorkspace := resolveWorkspacePath("workspaces/research")
	researchSkill := research.NewSkill()
	researchSkill.SetWorkspace(researchWorkspace)
	toolsRegistry.Register(researchSkill)

	chiefWorkspace := resolveWorkspacePath("workspaces/chief")
	chiefSkill := chief.NewSkill()
	chiefSkill.SetWorkspace(chiefWorkspace)
	toolsRegistry.Register(chiefSkill)

	atcWorkspace := resolveWorkspacePath("workspaces/atc")
	atcSkill := atc.NewSkill()
	atcSkill.SetWorkspace(atcWorkspace)
	toolsRegistry.Register(atcSkill)

	monitorSkill := monitor.NewSkill()
	monitorSkill.SetWorkspace(monitorWorkspace)
	toolsRegistry.Register(monitorSkill)

	coachWorkspace := resolveWorkspacePath("workspaces/coach")
	coachSkill := coach.NewSkill()
	coachSkill.SetWorkspace(coachWorkspace)
	toolsRegistry.Register(coachSkill)

	architectWorkspace := resolveWorkspacePath("workspaces/architect")
	architectSkill := architect.NewSkill()
	architectSkill.SetWorkspace(architectWorkspace)
	toolsRegistry.Register(architectSkill)

	subagentManager := subagent.NewSubagentManager(provider, workspace, nil)
	subagentManager.RegisterTool(researchSkill)
	subagentManager.RegisterTool(monitorSkill)
	subagentManager.RegisterTool(chiefSkill)
	subagentManager.RegisterTool(atcSkill)
	subagentManager.RegisterTool(coachSkill)
	subagentManager.RegisterTool(architectSkill)
	subagentTool := subagent.NewSubagentTool(subagentManager)
	toolsRegistry.Register(subagentTool)

	model := cfg.Agents.Defaults.Model
	if model == "" {
		model = "meta/llama-3.1-8b-instruct"
	}

	subagentManager.SetModel(model)
	message := ""
	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "-m" || os.Args[i] == "--message" {
			if i+1 < len(os.Args) {
				message = os.Args[i+1]
				i++
			}
		}
	}

	if message != "" {
		ctx := context.Background()
		response := processMessage(ctx, provider, model, toolsRegistry, message)
		fmt.Printf("\n%s %s\n\n", logo, response)
	} else {
		interactiveMode(provider, model, toolsRegistry)
	}
}

func processMessage(ctx context.Context, provider providers.LLMProvider, model string, toolsRegistry *tools.ToolRegistry, userMessage string) string {
	systemPrompt := `You are son-of-anthon, a personal multi-agent AI assistant.

Available tools (call as needed, including multiple times in one session):
- architect: Life admin; CalDAV sync/create/delete tasks on Nextcloud. Commands: sync_deadlines, create_task, delete_task
- chief: Strategic commander; reads daily briefs, urgent deadlines, morning/evening summaries. Commands: morning_brief, evening_review, urgent_deadlines, status, delegate
- atc: Task management; reads/writes tasks.xml, daily priorities. Commands: analyze_tasks, read_calendar, update_task, roll_over_tasks, sync_calendar, push_task
- coach: Learning coach; IELTS prep, habit tracking, Nextcloud integration. Commands: check_habits, fetch_material, generate_practice, evening_review, update_deck, nudge_telegram
- monitor: News curation; Bangladesh + Tech RSS feeds. Commands: fetch, status, feeds
- research: Academic paper discovery from ArXiv and HuggingFace. Commands: fetch
- subagent: Spawn any of the above as a dedicated subagent with deeper context

IMPORTANT RENDERING RULES:
- For morning_brief, evening_review, fetch news, search papers: reproduce the full tool output verbatim. Do NOT summarize or wrap in <status> tags.
- For create/delete/sync actions: a short confirmation is fine.
- When the user asks for a multi-step task, call tools sequentially as needed.`

	messages := []providers.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	toolDefs := toolsRegistry.ToProviderDefs()

	response, err := provider.Chat(ctx, messages, toolDefs, model, nil)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	if response == nil {
		return "Error: empty response from provider"
	}

	maxIterations := 8
	iterations := 0

	for len(response.ToolCalls) > 0 && iterations < maxIterations {
		iterations++
		messages = append(messages, providers.Message{
			Role:    "assistant",
			Content: response.Content,
		})

		for _, tc := range response.ToolCalls {
			toolName := tc.Name
			if toolName == "" && tc.Function != nil {
				toolName = tc.Function.Name
			}
			tcID := tc.ID

			if toolName == "" {
				continue
			}

			var args map[string]interface{}
			if len(tc.Arguments) > 0 {
				args = tc.Arguments
			} else if tc.Function != nil && tc.Function.Arguments != "" {
				json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}

			result := toolsRegistry.Execute(ctx, toolName, args)
			if result == nil {
				continue
			}

			log.Printf("[Main] Tool %s returned %d chars", toolName, len(result.ForUser))
			contentForLLM := result.ForLLM
			if contentForLLM == "" {
				contentForLLM = result.ForUser
			}
			messages = append(messages, providers.Message{
				Role:       "tool",
				Content:    contentForLLM,
				ToolCallID: tcID,
			})
		}

		response, err = provider.Chat(ctx, messages, toolDefs, model, nil)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		if response == nil {
			return "Error: empty response"
		}
		log.Printf("[Main] LLM response: %d chars, %d tool calls", len(response.Content), len(response.ToolCalls))
	}

	return response.Content
}

func interactiveMode(provider providers.LLMProvider, model string, toolsRegistry *tools.ToolRegistry) {
	fmt.Printf("%s Interactive mode (Ctrl+C to exit)\n", logo)
	fmt.Println("Type your message...")

	var input string
	for {
		fmt.Print("> ")
		fmt.Scanln(&input)
		if input == "exit" || input == "quit" {
			break
		}
		if input == "" {
			continue
		}

		ctx := context.Background()
		response := processMessage(ctx, provider, model, toolsRegistry, input)
		fmt.Printf("\n%s\n\n", response)
		input = ""
	}
}

func setupCronTool(agentLoop *agent.AgentLoop, msgBus *bus.MessageBus, workspace string, restrict bool, execTimeout time.Duration, config *config.Config) *cron.CronService {
	cronStorePath := filepath.Join(workspace, "cron", "jobs.json")
	cronService := cron.NewCronService(cronStorePath, nil)
	cronTool := tools.NewCronTool(cronService, agentLoop, msgBus, workspace, restrict, execTimeout, config)
	agentLoop.RegisterTool(cronTool)
	cronService.SetOnJob(func(job *cron.CronJob) (string, error) {
		result := cronTool.ExecuteJob(context.Background(), job)
		return result, nil
	})
	return cronService
}

func gatewayCmd() {
	args := os.Args[2:]
	for _, arg := range args {
		if arg == "--debug" || arg == "-d" {
			logger.SetLevel(logger.DEBUG)
			fmt.Println("🔍 Debug mode enabled")
			break
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	provider, err := providers.CreateProvider(cfg)
	if err != nil {
		fmt.Printf("Error creating provider: %v\n", err)
		os.Exit(1)
	}

	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)

	workspace := cfg.WorkspacePath()
	home, _ := os.UserHomeDir()
	globalWorkspace := filepath.Join(home, ".picoclaw", "workspace")

	if workspace == "" || workspace == "./workspaces/chief" {
		workspace = globalWorkspace
	}

	toolsRegistry := tools.NewToolRegistry()
	researchWorkspace := resolveWorkspacePath("workspaces/research")
	researchSkill := research.NewSkill()
	researchSkill.SetWorkspace(researchWorkspace)
	toolsRegistry.Register(researchSkill)
	agentLoop.RegisterTool(researchSkill)

	chiefWorkspace := resolveWorkspacePath("workspaces/chief")
	chiefSkill := chief.NewSkill()
	chiefSkill.SetWorkspace(chiefWorkspace)
	toolsRegistry.Register(chiefSkill)
	agentLoop.RegisterTool(chiefSkill)

	atcWorkspace := resolveWorkspacePath("workspaces/atc")
	atcSkill := atc.NewSkill()
	atcSkill.SetWorkspace(atcWorkspace)
	toolsRegistry.Register(atcSkill)
	agentLoop.RegisterTool(atcSkill)

	monitorWorkspace := resolveWorkspacePath("workspaces/monitor")
	monitorSkill := monitor.NewSkill()
	monitorSkill.SetWorkspace(monitorWorkspace)
	toolsRegistry.Register(monitorSkill)
	agentLoop.RegisterTool(monitorSkill)

	coachWorkspace := resolveWorkspacePath("workspaces/coach")
	coachSkill := coach.NewSkill()
	coachSkill.SetWorkspace(coachWorkspace)
	toolsRegistry.Register(coachSkill)
	agentLoop.RegisterTool(coachSkill)

	architectWorkspace := resolveWorkspacePath("workspaces/architect")
	architectSkill := architect.NewSkill()
	architectSkill.SetWorkspace(architectWorkspace)
	toolsRegistry.Register(architectSkill)
	agentLoop.RegisterTool(architectSkill)

	subagentManager := subagent.NewSubagentManager(provider, workspace, nil)
	subagentManager.RegisterTool(researchSkill)
	subagentManager.RegisterTool(monitorSkill)
	subagentManager.RegisterTool(chiefSkill)
	subagentManager.RegisterTool(atcSkill)
	subagentManager.RegisterTool(coachSkill)
	subagentManager.RegisterTool(architectSkill)
	subagentTool := subagent.NewSubagentTool(subagentManager)
	toolsRegistry.Register(subagentTool)
	agentLoop.RegisterTool(subagentTool)

	fmt.Println("\n📦 Agent Status:")
	startupInfo := agentLoop.GetStartupInfo()
	toolsInfo := startupInfo["tools"].(map[string]interface{})
	fmt.Printf("  • Tools: %d loaded\n", toolsInfo["count"])

	execTimeout := time.Duration(cfg.Tools.Cron.ExecTimeoutMinutes) * time.Minute
	cronService := setupCronTool(agentLoop, msgBus, workspace, cfg.Agents.Defaults.RestrictToWorkspace, execTimeout, cfg)

	heartbeatService := heartbeat.NewHeartbeatService(
		workspace,
		cfg.Heartbeat.Interval,
		cfg.Heartbeat.Enabled,
	)
	heartbeatService.SetBus(msgBus)

	heartbeatService.SetHandler(func(prompt, channel, chatID string) *tools.ToolResult {
		if channel == "" || chatID == "" {
			channel, chatID = "cli", "direct"
		}

		isUrgent := false

		deadlinesPath := filepath.Join(chiefWorkspace, "memory", "deadlines-today.md")
		if data, err := os.ReadFile(deadlinesPath); err == nil {
			content := string(data)
			if strings.Contains(content, "[P0]") || strings.Contains(content, "[P1]") || strings.Contains(content, "T00:00") {
				isUrgent = true
			}
		}

		tasksPath := filepath.Join(atcWorkspace, "tasks.xml")
		if data, err := os.ReadFile(tasksPath); err == nil {
			content := string(data)
			if (strings.Contains(content, "PRIORITY:0") || strings.Contains(content, "PRIORITY:1")) && !strings.Contains(content, "STATUS:COMPLETED") {
				isUrgent = true
			}
		}

		if !isUrgent {
			return tools.SilentResult("Heartbeat OK")
		}

		response, err := agentLoop.ProcessHeartbeat(context.Background(), prompt, channel, chatID)
		if err != nil {
			return tools.ErrorResult(fmt.Sprintf("Heartbeat error: %v", err))
		}
		if response == "HEARTBEAT_OK" {
			return tools.SilentResult("Heartbeat OK")
		}
		return tools.SilentResult(response)
	})

	channelManager, err := channels.NewManager(cfg, msgBus)
	if err != nil {
		fmt.Printf("Error creating channel manager: %v\n", err)
		os.Exit(1)
	}
	agentLoop.SetChannelManager(channelManager)

	var transcriber *voice.GroqTranscriber
	if cfg.Providers.Groq.APIKey != "" {
		transcriber = voice.NewGroqTranscriber(cfg.Providers.Groq.APIKey)
		logger.InfoC("voice", "Groq transcription enabled")
	}

	if transcriber != nil {
		if tc, ok := channelManager.GetChannel("telegram"); ok {
			if telegramChan, ok2 := tc.(*channels.TelegramChannel); ok2 {
				telegramChan.SetTranscriber(transcriber)
			}
		}
	}

	enabledChannels := channelManager.GetEnabledChannels()
	if len(enabledChannels) > 0 {
		fmt.Printf("✓ Channels enabled: %s\n", enabledChannels)
	} else {
		fmt.Println("⚠ Warning: No channels enabled")
	}

	fmt.Printf("✓ Gateway started on %s:%d\n", cfg.Gateway.Host, cfg.Gateway.Port)
	fmt.Println("Press Ctrl+C to stop")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cronService.Start(); err != nil {
		fmt.Printf("Error starting cron service: %v\n", err)
	} else {
		fmt.Println("✓ Cron service started")
	}

	if err := heartbeatService.Start(); err != nil {
		fmt.Printf("Error starting heartbeat service: %v\n", err)
	} else {
		fmt.Println("✓ Heartbeat service started")
	}

	stateManager := state.NewManager(workspace)
	deviceService := devices.NewService(devices.Config{
		Enabled:    cfg.Devices.Enabled,
		MonitorUSB: cfg.Devices.MonitorUSB,
	}, stateManager)
	deviceService.SetBus(msgBus)
	if err := deviceService.Start(ctx); err != nil {
		fmt.Printf("Error starting device service: %v\n", err)
	} else if cfg.Devices.Enabled {
		fmt.Println("✓ Device event service started")
	}

	if err := channelManager.StartAll(ctx); err != nil {
		fmt.Printf("Error starting channels: %v\n", err)
	}

	healthServer := health.NewServer(cfg.Gateway.Host, cfg.Gateway.Port)
	go func() {
		if err := healthServer.Start(); err != nil && err != http.ErrServerClosed {
			logger.ErrorCF("health", "Health server error", map[string]interface{}{"error": err.Error()})
		}
	}()

	go agentLoop.Run(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

	fmt.Println("\nShutting down...")
	cancel()
	healthServer.Stop(context.Background())
	deviceService.Stop()
	heartbeatService.Stop()
	cronService.Stop()
	agentLoop.Stop()
	channelManager.StopAll(ctx)
	fmt.Println("✓ Gateway stopped")
}
