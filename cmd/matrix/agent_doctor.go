package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentdoctor"
	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/logic/childidentity"
	"github.com/Josepavese/matrix/internal/logic/runaction"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/a2aclient"
	"github.com/Josepavese/matrix/internal/providers/agentprobe"
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

			endpoint := agentcfg.NormalizeEndpoint(cfg)
			address := agentdoctor.EndpointAddress(endpoint)
			resolved, policyErr := agentlaunch.ResolveEndpoint(id, endpoint)
			attachContext, attachNote := attachContextOf(ctx.Store, id, endpoint.Kind != "")

			item := map[string]any{
				"agent_id":            id,
				"kind":                endpoint.Kind,
				"transport":           endpoint.Transport,
				"address":             address,
				"command":             cfg.Command,
				"env_isolation":       cfg.EnvIsolation,
				"effective_active":    cfg.IsActive(),
				"effective_env_count": len(cfg.Env),
				"override_active":     override.Active,
				"override_env_count":  len(override.Env),
				"command_in_path":     false,
				"command_probe_ok":    false,
				// What is known about live context: the transport level from the
				// endpoint the agent resolved to, the provider and delivery levels
				// from the delivery records Matrix already wrote. The doctor runs no
				// attach, so an agent nothing was recorded for stays unobserved
				// rather than being credited with a capability it was never seen to
				// have.
				"attach_context":                attachContext,
				"provider_handshake_ok":         false,
				"provider_status":               "not_probed",
				"agents_config_path_override":   os.Getenv("MATRIX_AGENTS_CONFIG") != "",
				"telegram_config_path_override": os.Getenv("MATRIX_TELEGRAM_CONFIG") != "",
			}
			var warnings []string
			if attachNote != "" {
				warnings = append(warnings, attachNote)
			}
			if len(resolved.Metadata) > 0 {
				item["agent_launch_policy"] = resolved.Metadata
			}
			if policyErr != nil {
				item["provider_status"] = "launch_policy_invalid"
				item["agent_launch_policy_error"] = policyErr.Error()
				warnings = append(warnings, "agent launch policy is not applicable: "+policyErr.Error())
			} else {
				endpoint = resolved.Endpoint
			}

			if cfg.Command != "" {
				if _, err := os.Stat(cfg.Command); err == nil {
					item["command_in_path"] = true
				} else if _, err := execLookPath(cfg.Command); err == nil {
					item["command_in_path"] = true
				}
			}
			if policyErr == nil {
				checks, checkWarnings := agentdoctor.InspectACP(endpoint, agentprobe.ACPInitialize)
				for key, value := range checks {
					item[key] = value
				}
				warnings = append(warnings, checkWarnings...)
				if a2aAuth, a2aWarnings := agentdoctor.InspectA2AAuthentication(cmd.Context(), endpoint, a2aclient.FetchRemoteAuthCard); a2aAuth != nil {
					item["a2a_authentication"], warnings = a2aAuth, append(warnings, a2aWarnings...)
				}
				processCwd, cwdErr := childidentity.DeclaredProcessCwd(endpoint)
				if cwdErr != nil {
					item["provider_status"] = "process_cwd_invalid"
					item["process_cwd_error"] = cwdErr.Error()
					warnings = append(warnings, cwdErr.Error())
				} else {
					child, childWarnings := childidentity.Probe(endpoint, processCwd)
					if child.Status != "" {
						item["child"] = child
					}
					warnings = append(warnings, childWarnings...)
				}
			}
			meta, metaErr := agentcfg.LoadMeta(ctx.Store, id)
			if metaErr != nil {
				warnings = append(warnings, "agent metadata unavailable: "+metaErr.Error())
			}
			if meta.ArtifactVerification != nil {
				item["artifact_verification"] = meta.ArtifactVerification
				if meta.ArtifactVerification.Status == agentcfg.ArtifactDigestNotPublished {
					warnings = append(warnings, "installed artifact was not verified: the registry index publishes no sha256 for this platform")
				}
			}
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
			item["warnings"] = warnings

			report = append(report, item)
		}

		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			exitf("Error: %v", err)
		}
		// The report is this command's data output: it goes to stdout so a
		// caller can pipe it. cobra's Println writes to stderr when no out is
		// set, which made `matrix agent show <id> > file.json` produce an
		// empty file while the report looked present on a terminal.
		fmt.Fprintln(cmd.OutOrStdout(), string(out))
	},
}

// attachContextOf is the doctor's answer for one agent's live-context
// capability: the transport level from the endpoint the agent resolved to, and
// the provider and delivery levels from the delivery records Matrix already
// wrote while running that agent.
//
// A record that cannot be read is not an observation, and a record that holds no
// answer from the provider yet is not an acceptance: both leave the levels
// unknown, and the note names the record that was read so the generic
// "nothing was attempted" does not stand for something that did happen.
func attachContextOf(storage middleware.Storage, agentID string, transportAvailable bool) (agentdoctor.AttachContextCapability, string) {
	observation, err := runaction.LatestAttachObservation(storage, agentID)
	if err != nil {
		return agentdoctor.DescribeAttachContext(transportAvailable, agentdoctor.AttachObservation{}),
			"live-context delivery records could not be read: " + err.Error()
	}
	report := agentdoctor.DescribeAttachContext(transportAvailable, observation)
	if observation.Attempted || observation.DeliveryClass == "" {
		return report, ""
	}
	return report, "the last live-context attach is recorded as " + observation.DeliveryClass +
		", which says nothing yet about the provider's capability"
}

func init() {
	agentCmd.AddCommand(agentDoctorCmd)
}
