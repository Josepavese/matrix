package main

import (
	"encoding/json"
	"os"

	"github.com/jose/matrix-v2/internal/logic/agentcfg"
	"github.com/spf13/cobra"
)

var agentDoctorCmd = &cobra.Command{
	Use:   "doctor [agent_id]",
	Short: "Explain effective agent configuration and likely issues",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var target string
		if len(args) == 1 {
			target = args[0]
		}

		ctx, cleanup, err := NewAgentContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		ids := ctx.Registry.IDs()
		if target != "" {
			ids = []string{target}
		}

		report := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			cfg, err := ctx.Registry.Get(id)
			if err != nil {
				exitf("Error: %v", err)
			}
			override, err := agentcfg.Load(ctx.Store, id)
			if err != nil {
				exitf("Error: %v", err)
			}

			item := map[string]any{
				"agent_id":                      id,
				"protocol":                      cfg.Protocol,
				"command":                       cfg.Command,
				"env_isolation":                 cfg.EnvIsolation,
				"effective_active":              cfg.IsActive(),
				"effective_env_count":           len(cfg.Env),
				"override_active":               override.Active,
				"override_env_count":            len(override.Env),
				"command_in_path":               false,
				"agents_config_path_override":   os.Getenv("MATRIX_AGENTS_CONFIG") != "",
				"telegram_config_path_override": os.Getenv("MATRIX_TELEGRAM_CONFIG") != "",
			}

			if cfg.Command != "" {
				if _, err := os.Stat(cfg.Command); err == nil {
					item["command_in_path"] = true
				} else if _, err := execLookPath(cfg.Command); err == nil {
					item["command_in_path"] = true
				}
			}

			var warnings []string
			if !cfg.IsActive() {
				warnings = append(warnings, "agent disabled by effective configuration")
			}
			if cfg.Command == "" {
				warnings = append(warnings, "missing command")
			}
			if len(cfg.Env) == 0 {
				warnings = append(warnings, "no effective environment overrides")
			}
			item["warnings"] = warnings

			report = append(report, item)
		}

		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			exitf("Error: %v", err)
		}
		cmd.Println(string(out))
	},
}

func init() {
	agentCmd.AddCommand(agentDoctorCmd)
}
