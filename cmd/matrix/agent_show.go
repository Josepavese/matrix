package main

import (
	"encoding/json"
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/Josepavese/matrix/internal/logic/runtimecheck"
	"github.com/Josepavese/matrix/internal/middleware"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/network"
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
		endpoint := agentcfg.NormalizeEndpoint(agentcfg.Config{
			Command:         cfg.Command,
			Args:            cfg.Args,
			Env:             cfg.Env,
			Tenant:          cfg.Tenant,
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
		resolved, policyErr := agentlaunch.ResolveEndpoint(agentID, endpoint)

		payload := map[string]any{
			"agent_id":  agentID,
			"effective": cfg,
			"normalized_endpoint": map[string]any{
				"kind":             endpoint.Kind,
				"transport":        endpoint.Transport,
				"address":          address,
				"command":          endpoint.Command,
				"args":             endpoint.Args,
				"card_url":         endpoint.CardURL,
				"tenant":           endpoint.Tenant,
				"protocol_version": endpoint.ProtocolVersion,
			},
			"override":   override,
			"is_active":  cfg.IsActive(),
			"env_effect": cfg.Env,
		}
		// The runtime block is what tells a registered agent apart from a failed
		// one: it is read from the runtime's own record. An agent the runtime
		// starts per run is reported as served on demand, which is not a claim
		// that anybody watched it run, and a registration that does need an apply
		// is still reported as pending one.
		runtimeReport, runtimeErr := agentmgr.BuildRuntimeReport(agentmgr.RuntimeReportRequest{
			Store:    ctx.Store,
			Registry: ctx.Registry,
			Process:  execprovider.NewProvider(),
			CanDial:  func(address string) bool { return runtimecheck.CanDial(network.NewProvider(), address) },
			AgentID:  agentID,
		})
		if runtimeErr != nil {
			payload["runtime_error"] = runtimeErr.Error()
		} else {
			payload["runtime"] = runtimeReport
		}
		if len(resolved.Metadata) > 0 {
			payload["agent_launch_policy"] = resolved.Metadata
		}
		if policyErr != nil {
			payload["agent_launch_policy_error"] = policyErr.Error()
		} else if len(resolved.Metadata) > 0 {
			payload["launch_endpoint"] = map[string]any{
				"command":   resolved.Endpoint.Command,
				"args":      resolved.Endpoint.Args,
				"env_count": len(resolved.Endpoint.Env),
			}
		}
		out, err := json.MarshalIndent(payload, "", "  ")
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

func init() {
	agentCmd.AddCommand(agentShowCmd)
}
