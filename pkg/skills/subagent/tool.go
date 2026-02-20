package subagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/tools"
)

type SubagentTool struct {
	manager       *SubagentManager
	originChannel string
	originChatID  string
}

func NewSubagentTool(manager *SubagentManager) *SubagentTool {
	return &SubagentTool{
		manager:       manager,
		originChannel: "cli",
		originChatID:  "direct",
	}
}

func (t *SubagentTool) Name() string {
	return "subagent"
}

func (t *SubagentTool) Description() string {
	var agentList []string
	for agent, desc := range ValidAgents {
		agentList = append(agentList, fmt.Sprintf("%s: %s", agent, desc))
	}
	return fmt.Sprintf(`Execute a subagent task with agent-specific context. Available agent types:
- chief: Strategic commander, orchestrates other agents
- architect: Life admin, bills, medicine tracking  
- coach: Learning coach, IELTS prep, habit tracking
- monitor: News curation, Bangladesh + Tech news
- research: ArXiv/HuggingFace paper discovery
- atc: Task management, daily priorities

Each agent loads its own SOUL.md, AGENTS.md, TOOLS.md, and memory from its workspace.`)
}

func (t *SubagentTool) Parameters() map[string]interface{} {
	var agentOptions []string
	for agent := range ValidAgents {
		agentOptions = append(agentOptions, string(agent))
	}

	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The task for subagent to complete",
			},
			"agent_type": map[string]interface{}{
				"type":        "string",
				"description": "Agent type to use (chief, architect, coach, monitor, research, atc)",
				"enum":        agentOptions,
			},
			"label": map[string]interface{}{
				"type":        "string",
				"description": "Optional short label for the task (for tracking)",
			},
		},
		"required": []string{"task", "agent_type"},
	}
}

func (t *SubagentTool) SetContext(channel, chatID string) {
	t.originChannel = channel
	t.originChatID = chatID
}

func (t *SubagentTool) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	task, ok := args["task"].(string)
	if !ok {
		return tools.ErrorResult("task is required").WithError(fmt.Errorf("task parameter is required"))
	}

	agentTypeStr, ok := args["agent_type"].(string)
	if !ok {
		return tools.ErrorResult("agent_type is required").WithError(fmt.Errorf("agent_type parameter is required"))
	}

	agentType := AgentType(agentTypeStr)
	if _, valid := ValidAgents[agentType]; !valid {
		return tools.ErrorResult(fmt.Sprintf("invalid agent_type: %s", agentTypeStr)).WithError(fmt.Errorf("invalid agent_type"))
	}

	label, _ := args["label"].(string)

	if t.manager == nil {
		return tools.ErrorResult("Subagent manager not configured").WithError(fmt.Errorf("manager is nil"))
	}

	resultMsg, err := t.manager.Spawn(ctx, task, label, agentType, t.originChannel, t.originChatID)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Failed to spawn subagent: %v", err)).WithError(err)
	}

	userContent := resultMsg
	maxUserLen := 500
	if len(userContent) > maxUserLen {
		userContent = userContent[:maxUserLen] + "..."
	}

	llmContent := fmt.Sprintf("Subagent spawned:\nAgent Type: %s\nLabel: %s\nTask: %s\n\n%s",
		agentType, label, task, resultMsg)

	return &tools.ToolResult{
		ForLLM:  llmContent,
		ForUser: userContent,
		Silent:  false,
		IsError: false,
		Async:   false,
	}
}

func (t *SubagentTool) Help() string {
	var b strings.Builder
	b.WriteString("## Subagent Tool\n\n")
	b.WriteString("Use this tool to delegate tasks to specialized agents.\n\n")
	b.WriteString("### Agent Types:\n\n")
	for agent, desc := range ValidAgents {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", agent, desc))
	}
	b.WriteString("\n### Example:\n\n")
	b.WriteString("```json\n")
	b.WriteString(`{
  "task": "Find recent papers on GraphRAG",
  "agent_type": "research"
}`)
	b.WriteString("```\n")
	return b.String()
}
