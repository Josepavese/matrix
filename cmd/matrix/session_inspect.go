package main

import (
	"github.com/jose/matrix-v2/internal/logic/cmdutil"
	"github.com/jose/matrix-v2/internal/logic/session"
	"github.com/spf13/cobra"
)

var sessionInspectCmd = &cobra.Command{
	Use:   "inspect <session-id>",
	Short: "Inspect one logical session",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, closeFn, err := NewReadOnlyAppContext(DefaultVaultPath)
		if err != nil {
			exitf("failed to open vault: %v", err)
		}
		defer closeFn()

		mgr := session.NewManager(ctx.Store, nil, nil, nil)
		meta, found, err := mgr.InspectSession(args[0])
		if err != nil {
			exitf("failed to inspect session: %v", err)
		}
		if !found {
			exitf("session not found: %s", args[0])
		}
		if err := cmdutil.PrintJSON(cmd, meta); err != nil {
			exitf("failed to print session: %v", err)
		}
	},
}

func init() {
	sessionCmd.AddCommand(sessionInspectCmd)
}
