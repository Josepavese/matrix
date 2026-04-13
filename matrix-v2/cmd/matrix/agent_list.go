package main

import (
	"fmt"

	"github.com/jose/matrix-v2/internal/logic/agentcfg"
	"github.com/spf13/cobra"
)

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all known agents with effective active state",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cleanup, err := NewAgentContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		for _, id := range ctx.Registry.IDs() {
			cfg, err := ctx.Registry.Get(id)
			if err != nil {
				exitf("Error: %v", err)
			}
			state := "inactive"
			if cfg.IsActive() {
				state = "active"
			}

			meta, _ := agentcfg.LoadMeta(ctx.Store, id)
			desc := meta.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}

			fmt.Printf("%s\t%s\t%s\t%s\n", id, cfg.Protocol, state, desc)
		}
	},
}

func init() {
	agentCmd.AddCommand(agentListCmd)
}
