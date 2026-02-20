package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"

	"github.com/jony/son-of-anthon/pkg/skills/research"
	"github.com/jony/son-of-anthon/pkg/skills/subagent"
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
	fmt.Println("  version   Show version")
}

func loadConfig() (*config.Config, error) {
	home, _ := os.UserHomeDir()
	configPath := os.Getenv("PERSONAL_OS_CONFIG")
	if configPath == "" {
		configPath = fmt.Sprintf("%s/.picoclaw/config.json", home)
	}
	return config.LoadConfig(configPath)
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

	toolsRegistry := tools.NewToolRegistry()

	researchSkill := research.NewSkill()
	researchSkill.SetWorkspace(workspace)
	toolsRegistry.Register(researchSkill)

	subagentManager := subagent.NewSubagentManager(provider, workspace, nil)
	subagentTool := subagent.NewSubagentTool(subagentManager)
	toolsRegistry.Register(subagentTool)

	model := cfg.Agents.Defaults.Model
	if model == "" {
		model = "meta/llama-3.1-8b-instruct"
	}

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
	systemPrompt := `You are a helpful AI assistant with access to tools.

Available tools:
- research: Search for academic papers on ArXiv and HuggingFace. Returns JSON with papers list.
- subagent: Spawn specialized subagents (chief, architect, coach, monitor, research, atc)

IMPORTANT: After getting tool results, provide your final answer to the user. Do NOT call tools repeatedly for the same task.`

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

	maxIterations := 3
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

			messages = append(messages, providers.Message{
				Role:       "tool",
				Content:    result.ForUser,
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
