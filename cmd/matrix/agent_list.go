package main

import (
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/spf13/cobra"
)

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all known agents with effective active state",
	Args:  cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
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
			endpoint := agentcfg.NormalizeEndpoint(agentcfg.Config{
				Command:         cfg.Command,
				Args:            cfg.Args,
				Env:             cfg.Env,
				Kind:            cfg.Kind,
				Transport:       cfg.Transport,
				Address:         cfg.Address,
				CardURL:         cfg.CardURL,
				ProtocolVersion: cfg.ProtocolVersion,
				HealthcheckPath: cfg.HealthcheckPath,
				EnvIsolation:    cfg.EnvIsolation,
				Active:          cfg.Active,
			})
			state := "inactive"
			if cfg.IsActive() {
				state = "active"
			}

			meta, _ := agentcfg.LoadMeta(ctx.Store, id)
			desc := meta.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}

			address := endpoint.Address
			if endpoint.Kind == middleware.ProtocolKindACP && endpoint.Transport == "stdio" {
				address = endpoint.Command
			}
			fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n", id, endpoint.Kind, endpoint.Transport, state, address, desc)
		}
	},
}

func init() {
	agentCmd.AddCommand(agentListCmd)
}
