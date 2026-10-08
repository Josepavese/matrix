package main

import (
	"fmt"
	"path"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/spf13/cobra"
)

var sandboxCommandArgs []string
var sandboxCommandCmd = &cobra.Command{
	Use: "command <agent_id> <absolute-image-command>", Short: "Set an ACP command inside an explicitly configured container image", Args: cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		if !path.IsAbs(args[1]) || strings.Contains(args[1], "\\") {
			return fmt.Errorf("container command must be an absolute Linux image path")
		}
		app, closeFn, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			return err
		}
		defer closeFn()
		cfg, err := app.Registry.Get(args[0])
		if err != nil {
			return err
		}
		policy, err := agentlaunch.ReadSandbox(agentcfg.NormalizeEndpoint(cfg))
		if err != nil {
			return err
		}
		if policy == nil || policy.Container == nil {
			return fmt.Errorf("configure MATRIX_SANDBOX with a container contract before its image command")
		}
		entry, err := agentcfg.LoadEntry(app.Store, args[0])
		if err != nil {
			return err
		}
		entry.Config.Command, entry.Config.Args = args[1], append([]string{}, sandboxCommandArgs...)
		entry.Config.Kind, entry.Config.Transport = "acp", "stdio"
		entry.Config.EnvIsolation = false
		return agentcfg.SaveEntry(app.Store, args[0], entry)
	},
}

func init() {
	sandboxCommandCmd.Flags().StringArrayVar(&sandboxCommandArgs, "arg", nil, "image command argument, repeated in order")
	sandboxCmd.AddCommand(sandboxCommandCmd)
}
