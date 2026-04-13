package main

import (
	"fmt"

	"github.com/jose/matrix-v2/internal/logic/agentcfg"
	"github.com/spf13/cobra"
)

var agentEnableCmd = &cobra.Command{
	Use:   "enable <agent_id>",
	Short: "Enable an agent via SSOT override",
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
		active := true
		override.Active = &active
		if err := agentcfg.Save(ctx.Store, agentID, override); err != nil {
			exitf("Error: %v", err)
		}
		fmt.Printf("Agent %s enabled in SSOT override\n", agentID)
	},
}

var agentDisableCmd = &cobra.Command{
	Use:   "disable <agent_id>",
	Short: "Disable an agent via SSOT override",
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
		active := false
		override.Active = &active
		if err := agentcfg.Save(ctx.Store, agentID, override); err != nil {
			exitf("Error: %v", err)
		}
		fmt.Printf("Agent %s disabled in SSOT override\n", agentID)
	},
}

func init() {
	agentCmd.AddCommand(agentEnableCmd)
	agentCmd.AddCommand(agentDisableCmd)
}
