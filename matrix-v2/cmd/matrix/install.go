package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jose/matrix-v2/internal/logic/agentmgr"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	networkprovider "github.com/jose/matrix-v2/internal/providers/network"
	"github.com/jose/matrix-v2/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install [agent_id]",
	Short: "Install an AI agent from the ACP Registry",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		agentID := args[0]

		// 1. Setup Dependencies
		provider, err := bolt.NewProvider(DefaultVaultPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault error: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = provider.Close() }()

		netProv := networkprovider.NewProvider()
		archiveProv := osfs.NewArchiveProvider()
		regClient := agentmgr.NewCachingRegistryClient(netProv, "", provider)

		installer, err := agentmgr.NewInstaller(agentmgr.InstallerConfig{
			Net:      netProv,
			Archive:  archiveProv,
			Storage:  provider,
			FS:       osfs.NewFSProvider(),
			Registry: regClient,
			BaseDir:  "",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Installer init error: %v\n", err)
			os.Exit(1)
		}

		// 2. Execute Install
		ctx := context.Background()
		if err := installer.Install(ctx, agentID); err != nil {
			fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully installed and registered agent '%s'\n", agentID)
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
