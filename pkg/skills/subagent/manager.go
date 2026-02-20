package subagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

type AgentType string

const (
	AgentChief     AgentType = "chief"
	AgentArchitect AgentType = "architect"
	AgentCoach     AgentType = "coach"
	AgentMonitor   AgentType = "monitor"
	AgentResearch  AgentType = "research"
	AgentATC       AgentType = "atc"
)

var ValidAgents = map[AgentType]string{
	AgentChief:     "Strategic commander, orchestrates other agents",
	AgentArchitect: "Life admin, bills, medicine tracking",
	AgentCoach:     "Learning coach, IELTS prep, habit tracking",
	AgentMonitor:   "News curation, Bangladesh + Tech",
	AgentResearch:  "ArXiv/HuggingFace paper discovery",
	AgentATC:       "Task management, daily priorities",
}

type SubagentConfig struct {
	AgentType     AgentType
	WorkspacePath string
	Model         string
	MaxTokens     int
	Temperature   float64
	MaxIterations int
}

type SubagentTask struct {
	ID            string
	Task          string
	Label         string
	AgentType     AgentType
	OriginChannel string
	OriginChatID  string
	Status        string
	Result        string
	Iterations    int
	Created       int64
}

type SubagentManager struct {
	tasks         map[string]*SubagentTask
	mu            sync.RWMutex
	provider      providers.LLMProvider
	config        SubagentConfig
	bus           *bus.MessageBus
	workspaceBase string
	tools         *tools.ToolRegistry
	nextID        int
}

func NewSubagentManager(provider providers.LLMProvider, workspaceBase string, bus *bus.MessageBus) *SubagentManager {
	return &SubagentManager{
		tasks:         make(map[string]*SubagentTask),
		provider:      provider,
		bus:           bus,
		workspaceBase: workspaceBase,
		tools:         tools.NewToolRegistry(),
		config: SubagentConfig{
			Model:         "google-antigravity/gemini-3-flash",
			MaxTokens:     8192,
			Temperature:   0.7,
			MaxIterations: 10,
		},
		nextID: 1,
	}
}

func (sm *SubagentManager) SetModel(model string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.config.Model = model
}

func (sm *SubagentManager) SetMaxTokens(maxTokens int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.config.MaxTokens = maxTokens
}

func (sm *SubagentManager) SetTemperature(temp float64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.config.Temperature = temp
}

func (sm *SubagentManager) RegisterTool(tool tools.Tool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.tools.Register(tool)
}

func (sm *SubagentManager) Spawn(ctx context.Context, task, label string, agentType AgentType, originChannel, originChatID string) (string, error) {
	sm.mu.Lock()
	taskID := fmt.Sprintf("subagent-%d", sm.nextID)
	sm.nextID++

	subagentTask := &SubagentTask{
		ID:            taskID,
		Task:          task,
		Label:         label,
		AgentType:     agentType,
		OriginChannel: originChannel,
		OriginChatID:  originChatID,
		Status:        "running",
	}
	sm.tasks[taskID] = subagentTask
	sm.mu.Unlock()

	go sm.runTask(ctx, subagentTask)

	if label != "" {
		return fmt.Sprintf("Spawned subagent '%s' (%s) for task: %s", label, agentType, task), nil
	}
	return fmt.Sprintf("Spawned subagent (%s) for task: %s", agentType, task), nil
}

func (sm *SubagentManager) runTask(ctx context.Context, task *SubagentTask) {
	task.Status = "running"

	workspacePath := sm.getWorkspacePath(task.AgentType)
	systemPrompt := sm.buildSystemPrompt(workspacePath, task.AgentType)

	messages := []providers.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: task.Task},
	}

	sm.mu.RLock()
	cfg := sm.config
	toolReg := sm.tools
	sm.mu.RUnlock()

	var llmOptions map[string]any
	if cfg.MaxTokens > 0 || cfg.Temperature > 0 {
		llmOptions = map[string]any{}
		if cfg.MaxTokens > 0 {
			llmOptions["max_tokens"] = cfg.MaxTokens
		}
		if cfg.Temperature > 0 {
			llmOptions["temperature"] = cfg.Temperature
		}
	}

	result, err := tools.RunToolLoop(ctx, tools.ToolLoopConfig{
		Provider:      sm.provider,
		Model:         cfg.Model,
		Tools:         toolReg,
		MaxIterations: cfg.MaxIterations,
		LLMOptions:    llmOptions,
	}, messages, task.OriginChannel, task.OriginChatID)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if err != nil {
		task.Status = "failed"
		task.Result = fmt.Sprintf("Error: %v", err)
	} else {
		task.Status = "completed"
		task.Result = result.Content
		task.Iterations = result.Iterations
	}

	if sm.bus != nil {
		announceContent := fmt.Sprintf("Task '%s' (%s) completed.\n\nResult:\n%s",
			task.Label, task.AgentType, task.Result)
		sm.bus.PublishInbound(bus.InboundMessage{
			Channel:  "system",
			SenderID: fmt.Sprintf("subagent:%s", task.ID),
			ChatID:   fmt.Sprintf("%s:%s", task.OriginChannel, task.OriginChatID),
			Content:  announceContent,
		})
	}
}

func (sm *SubagentManager) getWorkspacePath(agentType AgentType) string {
	if sm.workspaceBase == "" {
		return ""
	}
	return filepath.Join(sm.workspaceBase, string(agentType))
}

func (sm *SubagentManager) buildSystemPrompt(workspacePath string, agentType AgentType) string {
	var prompt strings.Builder

	prompt.WriteString("You are a subagent. Complete the given task independently and report the result.\n")
	prompt.WriteString("You have access to tools - use them as needed to complete your task.\n")
	prompt.WriteString("After completing the task, provide a clear summary of what was done.\n\n")

	if workspacePath != "" {
		if soulPath := filepath.Join(workspacePath, "SOUL.md"); fileExists(soulPath) {
			if content, err := os.ReadFile(soulPath); err == nil {
				prompt.WriteString("## Your SOUL (Personality)\n\n")
				prompt.WriteString(string(content))
				prompt.WriteString("\n\n")
			}
		}

		if agentsPath := filepath.Join(workspacePath, "AGENTS.md"); fileExists(agentsPath) {
			if content, err := os.ReadFile(agentsPath); err == nil {
				prompt.WriteString("## Your Instructions\n\n")
				prompt.WriteString(string(content))
				prompt.WriteString("\n\n")
			}
		}

		if toolsPath := filepath.Join(workspacePath, "TOOLS.md"); fileExists(toolsPath) {
			if content, err := os.ReadFile(toolsPath); err == nil {
				prompt.WriteString("## Available Tools\n\n")
				prompt.WriteString(string(content))
				prompt.WriteString("\n\n")
			}
		}

		if userPath := filepath.Join(workspacePath, "USER.md"); fileExists(userPath) {
			if content, err := os.ReadFile(userPath); err == nil {
				prompt.WriteString("## User Context\n\n")
				prompt.WriteString(string(content))
				prompt.WriteString("\n\n")
			}
		}

		memoryPath := filepath.Join(workspacePath, "memory")
		if memPath := filepath.Join(memoryPath, "MEMORY.md"); fileExists(memPath) {
			if content, err := os.ReadFile(memPath); err == nil {
				prompt.WriteString("## Long-term Memory\n\n")
				prompt.WriteString(string(content))
				prompt.WriteString("\n\n")
			}
		}
	}

	return prompt.String()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (sm *SubagentManager) GetTask(taskID string) (*SubagentTask, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	task, ok := sm.tasks[taskID]
	return task, ok
}

func (sm *SubagentManager) ListTasks() []*SubagentTask {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	tasks := make([]*SubagentTask, 0, len(sm.tasks))
	for _, task := range sm.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}
