package main

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/containersandbox"
	"github.com/spf13/cobra"
)

var sandboxWorkspacePath string

var sandboxCmd = &cobra.Command{Use: "sandbox", Short: "Inspect explicitly configured provider and OS sandbox contracts"}
var sandboxDoctorCmd = &cobra.Command{
	Use: "doctor <agent_id>", Short: "Check sandbox declaration and installed prerequisites without launching an agent", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, closeFn, err := NewReadOnlyAppContext(DefaultVaultPath)
		if err != nil {
			return err
		}
		defer closeFn()
		registry, err := agentmgr.NewRegistry(app.ConfigRdr, app.Store)
		if err != nil {
			return err
		}
		cfg, err := registry.Get(args[0])
		if err != nil {
			return err
		}
		resolved, err := agentlaunch.ResolveEndpoint(args[0], agentcfg.NormalizeEndpoint(cfg))
		if err != nil {
			return err
		}
		result := map[string]interface{}{"platform": runtime.GOOS, "agent_id": args[0], "status": "not_requested"}
		policy := resolved.Endpoint.Sandbox
		if policy != nil {
			result["status"] = "configured_not_launched"
			result["policy"] = resolved.Metadata["sandbox"]
		}
		if policy != nil && policy.Container != nil {
			path, pathErr := resolveInvocationPath(sandboxWorkspacePath)
			if pathErr != nil {
				return pathErr
			}
			result["os_prerequisites"], err = containersandbox.Probe(cmd.Context(), containersandbox.Launch{Policy: *policy.Container, Workspace: path, Identity: args[0], Command: resolved.Endpoint.Command})
		}
		data, encodeErr := json.MarshalIndent(result, "", "  ")
		if encodeErr != nil {
			return encodeErr
		}
		if _, writeErr := fmt.Fprintln(cmd.OutOrStdout(), string(data)); writeErr != nil {
			return writeErr
		}
		return err
	},
}

func init() {
	sandboxDoctorCmd.Flags().StringVar(&sandboxWorkspacePath, "workspace", "", "existing host workspace for container prerequisite checks")
	sandboxCmd.AddCommand(sandboxDoctorCmd)
	rootCmd.AddCommand(sandboxCmd)
}

func doctorContainerEndpoint(endpoint middleware.ProtocolEndpoint) bool {
	return endpoint.Sandbox != nil && endpoint.Sandbox.Container != nil
}
