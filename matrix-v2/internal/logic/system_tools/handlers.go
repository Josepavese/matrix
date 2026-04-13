package system_tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jose/matrix-v2/internal/logic/agentcfg"
	"github.com/jose/matrix-v2/internal/logic/agentmgr"
	"github.com/jose/matrix-v2/internal/middleware"
)

// GetSystemTools returns the available system tools for the Meta-Agent
func GetSystemTools() []middleware.Tool {
	return []middleware.Tool{
		{
			Name:        "APM_Install",
			Description: "Installs and enables an AI agent locally in the Matrix OS. Use this when the user asks to install, add, or download an agent. Valid agents: claude, gemini, opencode, codex, kimi.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"agent": map[string]interface{}{
						"type":        "string",
						"description": "The name of the agent to install (e.g. claude)",
					},
					"apiKey": map[string]interface{}{
						"type":        "string",
						"description": "Optional API key if provided by the user",
					},
				},
				"required": []string{"agent"},
			},
		},
		{
			Name:        "Config_Set",
			Description: "Updates a system configuration value or agent setting.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key": map[string]interface{}{
						"type":        "string",
						"description": "The configuration key to set",
					},
					"value": map[string]interface{}{
						"type":        "string",
						"description": "The configuration value or JSON string to set",
					},
				},
				"required": []string{"key", "value"},
			},
		},
		{
			Name:        "APM_Uninstall",
			Description: "Uninstalls or disables an AI agent from the Matrix OS.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"agent": map[string]interface{}{
						"type":        "string",
						"description": "The name of the agent to uninstall",
					},
				},
				"required": []string{"agent"},
			},
		},
	}
}

type Handler struct {
	config    middleware.ConfigManager
	storage   middleware.Storage
	installer *agentmgr.Installer
}

func NewHandler(config middleware.ConfigManager, storage middleware.Storage, installer *agentmgr.Installer) *Handler {
	return &Handler{config: config, storage: storage, installer: installer}
}

// ExecuteTool routes the tool execution based on method name and payload
func (h *Handler) ExecuteTool(call middleware.ToolCall) string {
	switch call.Function.Name {
	case "APM_Install":
		var args struct {
			Agent  string `json:"agent"`
			APIKey string `json:"apiKey"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Error parsing APM_Install arguments: %v", err)
		}
		return h.handleAPMInstall(args.Agent, args.APIKey)

	case "APM_Uninstall":
		var args struct {
			Agent string `json:"agent"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Error parsing APM_Uninstall arguments: %v", err)
		}
		return h.handleAPMUninstall(args.Agent)

	case "Config_Set":
		var args struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Error parsing Config_Set arguments: %v", err)
		}
		if err := h.config.WriteConfig(args.Key, []byte(args.Value)); err != nil {
			return fmt.Sprintf("Error setting config %s: %v", args.Key, err)
		}
		return fmt.Sprintf("Success: Set config %s", args.Key)

	default:
		return fmt.Sprintf("Unknown tool: %s", call.Function.Name)
	}
}

// Below are the underlying executable logic for System Tools

func (h *Handler) handleAPMInstall(agentName, apiKey string) string {
	// First, check if already in Vault
	ids, err := agentcfg.ListAgentIDs(h.storage)
	if err != nil {
		return fmt.Sprintf("Error listing agents: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == agentName {
			found = true
			break
		}
	}

	if !found {
		// Attempt dynamic installation
		if h.installer != nil {
			ctx := context.Background()
			if err := h.installer.Install(ctx, agentName); err != nil {
				return fmt.Sprintf("Failed to install agent '%s' from registry: %v", agentName, err)
			}
		} else {
			return fmt.Sprintf("Agent '%s' not found and installer not available.", agentName)
		}
	}

	// Update override
	override, err := agentcfg.Load(h.storage, agentName)
	if err != nil {
		return fmt.Sprintf("Error reading agent override: %v", err)
	}
	active := true
	override.Active = &active
	if apiKey != "" {
		override.Env = agentcfg.UpsertEnv(override.Env, "API_KEY", apiKey)
	}

	if err := agentcfg.Save(h.storage, agentName, override); err != nil {
		return fmt.Sprintf("Error writing agent override: %v", err)
	}

	return fmt.Sprintf("Successfully installed and activated agent: %s", agentName)
}

func (h *Handler) handleAPMUninstall(agentName string) string {
	override, err := agentcfg.Load(h.storage, agentName)
	if err != nil {
		return fmt.Sprintf("Error reading agent override or agent not found: %v", err)
	}
	active := false
	override.Active = &active
	if err := agentcfg.Save(h.storage, agentName, override); err != nil {
		return fmt.Sprintf("Error writing agent override: %v", err)
	}

	return fmt.Sprintf("Successfully deactivated agent: %s", agentName)
}
