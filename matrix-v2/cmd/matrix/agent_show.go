package main

import (
	"encoding/json"

	"github.com/jose/matrix-v2/internal/logic/agentcfg"
	"github.com/spf13/cobra"
)

var agentShowCmd = &cobra.Command{
	Use:   "show <agent_id>",
	Short: "Show effective and override configuration for an agent",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		agentID := args[0]

		ctx, cleanup, err := NewAgentContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		cfg, err := ctx.Registry.Get(agentID)
		if err != nil {
			exitf("Error: %v", err)
		}
		override, err := agentcfg.Load(ctx.Store, agentID)
		if err != nil {
			exitf("Error: %v", err)
		}

		payload := map[string]any{
			"agent_id":   agentID,
			"effective":  cfg,
			"override":   override,
			"is_active":  cfg.IsActive(),
			"env_effect": cfg.Env,
		}
		out, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			exitf("Error: %v", err)
		}
		cmd.Println(string(out))
	},
}

func init() {
	agentCmd.AddCommand(agentShowCmd)
}
