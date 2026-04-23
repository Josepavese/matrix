package main

import (
	"encoding/json"
	"os"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentdoctor"
	"github.com/Josepavese/matrix/internal/middleware"
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
			address := endpoint.Address
			if endpoint.Kind == middleware.ProtocolKindACP && endpoint.Transport == "stdio" {
				address = endpoint.Command
			}

			item := map[string]any{
				"agent_id":                      id,
				"kind":                          endpoint.Kind,
				"transport":                     endpoint.Transport,
				"address":                       address,
				"command":                       cfg.Command,
				"env_isolation":                 cfg.EnvIsolation,
				"effective_active":              cfg.IsActive(),
				"effective_env_count":           len(cfg.Env),
				"override_active":               override.Active,
				"override_env_count":            len(override.Env),
				"command_in_path":               false,
				"command_probe_ok":              false,
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
			if endpoint.Kind == middleware.ProtocolKindACP && endpoint.Transport == "stdio" && endpoint.Command != "" {
				probe := agentdoctor.ProbeCommand(endpoint.Command, endpoint.Args, cfg.Env, cfg.EnvIsolation)
				item["command_probe_ok"] = probe.OK
				item["command_probe_exit_code"] = probe.ExitCode
				if probe.Error != "" {
					item["command_probe_error"] = probe.Error
				}
			}

			var warnings []string
			if !cfg.IsActive() {
				warnings = append(warnings, "agent disabled by effective configuration")
			}
			switch endpoint.Kind {
			case middleware.ProtocolKindACP:
				if endpoint.Transport == "stdio" && endpoint.Command == "" {
					warnings = append(warnings, "missing local command for ACP stdio endpoint")
				}
				if endpoint.Transport != "stdio" && endpoint.Address == "" {
					warnings = append(warnings, "missing remote address for ACP endpoint")
				}
			case middleware.ProtocolKindA2A:
				if endpoint.Address == "" && endpoint.CardURL == "" && endpoint.Command == "" {
					warnings = append(warnings, "missing address, card_url, or local command for A2A endpoint")
				}
			default:
				warnings = append(warnings, "unknown protocol kind")
			}
			if len(cfg.Env) == 0 {
				warnings = append(warnings, "no effective environment overrides")
			}
			if probeOK, _ := item["command_probe_ok"].(bool); endpoint.Kind == middleware.ProtocolKindACP && endpoint.Transport == "stdio" && !probeOK {
				warnings = append(warnings, "ACP stdio command probe failed")
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
