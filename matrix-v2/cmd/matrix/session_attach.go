package main

import (
	"fmt"
	"os"

	"github.com/jose/matrix-v2/internal/logic/session"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	"github.com/spf13/cobra"
)

var sessionAttachCmd = &cobra.Command{
	Use:   "attach [channel_id] [session_id]",
	Short: "Attach a channel to an existing session",
	Args:  cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		channelID := args[0]
		sessionID := args[1]

		provider, err := bolt.NewProvider(DefaultVaultPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault error: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = provider.Close() }()

		mgr := session.NewManager(provider, nil, nil, nil) // AgentRouter, Wizard, SystemTools not needed for attaching
		err = mgr.AttachChannel(channelID, sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to attach channel: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Channel %s linked to session %s\n", channelID, sessionID)
	},
}

func init() {
	sessionCmd.AddCommand(sessionAttachCmd)
}
