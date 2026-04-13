package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/agentmgr"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	networkprovider "github.com/jose/matrix-v2/internal/providers/network"
	"github.com/spf13/cobra"
)

var agentInfoCmd = &cobra.Command{
	Use:   "info <agent_id>",
	Short: "Show remote agent details from the ACP Registry",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		agentID := args[0]

		provider, err := bolt.NewProvider(DefaultVaultPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault error: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = provider.Close() }()

		netProv := networkprovider.NewProvider()
		regClient := agentmgr.NewCachingRegistryClient(netProv, "", provider)

		ctx := context.Background()
		manifest, err := regClient.FetchManifestCached(ctx, agentID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("ID:           %s\n", manifest.ID)
		fmt.Printf("Name:         %s\n", manifest.Name)
		fmt.Printf("Version:      %s\n", manifest.Version)
		fmt.Printf("Description:  %s\n", manifest.Description)
		if manifest.Repository != "" {
			fmt.Printf("Repository:   %s\n", manifest.Repository)
		}
		if manifest.Website != "" {
			fmt.Printf("Website:      %s\n", manifest.Website)
		}
		if len(manifest.Authors) > 0 {
			fmt.Printf("Authors:      %s\n", strings.Join(manifest.Authors, ", "))
		}
		if manifest.License != "" {
			fmt.Printf("License:      %s\n", manifest.License)
		}
		fmt.Printf("Distribution: %s\n", strings.Join(manifest.DistTypes(), ", "))
	},
}

func init() {
	agentCmd.AddCommand(agentInfoCmd)
}
