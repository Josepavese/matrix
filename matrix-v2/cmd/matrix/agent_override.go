package main

import (
	"encoding/json"
	"fmt"

	"github.com/jose/matrix-v2/internal/logic/agentcfg"
	"github.com/spf13/cobra"
)

var agentOverrideCmd = &cobra.Command{
	Use:   "override",
	Short: "Inspect and manage raw agent SSOT overrides",
}

var agentOverrideListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agents that currently have an SSOT override",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cleanup, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		ids, err := agentcfg.ListAgentIDs(ctx.Store)
		if err != nil {
			exitf("Error: %v", err)
		}
		for _, id := range ids {
			fmt.Println(id)
		}
	},
}

var agentOverrideShowCmd = &cobra.Command{
	Use:   "show <agent_id>",
	Short: "Show the raw SSOT override for an agent",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		agentID := args[0]
		ctx, cleanup, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		override, err := agentcfg.Load(ctx.Store, agentID)
		if err != nil {
			exitf("Error: %v", err)
		}
		out, err := json.MarshalIndent(map[string]any{
			"agent_id": agentID,
			"override": override,
		}, "", "  ")
		if err != nil {
			exitf("Error: %v", err)
		}
		cmd.Println(string(out))
	},
}

var agentOverrideClearCmd = &cobra.Command{
	Use:   "clear <agent_id>",
	Short: "Remove all SSOT overrides for an agent",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		agentID := args[0]
		ctx, cleanup, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		if err := agentcfg.DeleteEntry(ctx.Store, agentID); err != nil {
			exitf("Error: %v", err)
		}
		fmt.Printf("Agent %s SSOT override cleared\n", agentID)
	},
}

func init() {
	agentOverrideCmd.AddCommand(agentOverrideListCmd)
	agentOverrideCmd.AddCommand(agentOverrideShowCmd)
	agentOverrideCmd.AddCommand(agentOverrideClearCmd)
	agentCmd.AddCommand(agentOverrideCmd)
}
