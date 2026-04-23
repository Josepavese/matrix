package main

import (
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/spf13/cobra"
)

var agentEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage agent environment overrides stored in SSOT",
}

var agentEnvListCmd = &cobra.Command{
	Use:   "list <agent_id>",
	Short: "List SSOT environment overrides for an agent",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
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
		for _, env := range override.Env {
			fmt.Println(env)
		}
	},
}

var agentEnvSetCmd = &cobra.Command{
	Use:   "set <agent_id> <key> <value>",
	Short: "Set an agent environment override in SSOT",
	Args:  cobra.ExactArgs(3),
	Run: func(_ *cobra.Command, args []string) {
		agentID, key, value := args[0], args[1], args[2]
		ctx, cleanup, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		override, err := agentcfg.Load(ctx.Store, agentID)
		if err != nil {
			exitf("Error: %v", err)
		}
		override.Env = agentcfg.UpsertEnv(override.Env, key, value)
		if err := agentcfg.Save(ctx.Store, agentID, override); err != nil {
			exitf("Error: %v", err)
		}
		fmt.Printf("Agent %s env %s updated in SSOT override\n", agentID, key)
	},
}

var agentEnvUnsetCmd = &cobra.Command{
	Use:   "unset <agent_id> <key>",
	Short: "Remove an agent environment override from SSOT",
	Args:  cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		agentID, key := args[0], args[1]
		ctx, cleanup, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		override, err := agentcfg.Load(ctx.Store, agentID)
		if err != nil {
			exitf("Error: %v", err)
		}
		override.Env = agentcfg.RemoveEnv(override.Env, key)
		if err := agentcfg.Save(ctx.Store, agentID, override); err != nil {
			exitf("Error: %v", err)
		}
		fmt.Printf("Agent %s env %s removed from SSOT override\n", agentID, key)
	},
}

func init() {
	agentEnvCmd.AddCommand(agentEnvListCmd)
	agentEnvCmd.AddCommand(agentEnvSetCmd)
	agentEnvCmd.AddCommand(agentEnvUnsetCmd)
	agentCmd.AddCommand(agentEnvCmd)
}
