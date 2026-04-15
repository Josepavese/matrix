package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/agentdiscovery"
	networkprovider "github.com/jose/matrix-v2/internal/providers/network"
	"github.com/spf13/cobra"
)

var (
	agentInfoSource     string
	agentInfoCatalogURL string
)

var agentInfoCmd = &cobra.Command{
	Use:   "info <reference>",
	Short: "Show agent details from a discovery source",
	Long:  "Show details from the local SSOT, the ACP registry, an A2A agent card URL, or an A2A catalog entry.",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		ref := args[0]
		source := agentdiscovery.Source(agentInfoSource)

		opts := agentdiscovery.Options{
			Net:        networkprovider.NewProvider(),
			CatalogURL: agentInfoCatalogURL,
		}
		if source == agentdiscovery.SourceLocal || source == agentdiscovery.SourceACPRegistry {
			agentCtx, cleanup, err := NewAgentContext(DefaultVaultPath)
			if err != nil {
				exitf("Error: %v", err)
			}
			defer cleanup()
			opts.Storage = agentCtx.Store
			opts.Registry = agentCtx.Registry
		}

		provider, err := agentdiscovery.NewProvider(source, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		record, err := provider.Get(context.Background(), ref)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("ID:           %s\n", record.ID)
		fmt.Printf("Name:         %s\n", record.Name)
		fmt.Printf("Version:      %s\n", record.Version)
		fmt.Printf("Source:       %s\n", record.Source)
		fmt.Printf("Kind:         %s\n", record.Kind)
		if record.ProtocolVersion != "" {
			fmt.Printf("Protocol Ver: %s\n", record.ProtocolVersion)
		}
		if record.Transport != "" {
			fmt.Printf("Transport:    %s\n", record.Transport)
		}
		fmt.Printf("Description:  %s\n", record.Description)
		if record.Address != "" {
			fmt.Printf("Address:      %s\n", record.Address)
		}
		if record.CardURL != "" {
			fmt.Printf("Card URL:     %s\n", record.CardURL)
		}
		if record.Repository != "" {
			fmt.Printf("Repository:   %s\n", record.Repository)
		}
		if record.Website != "" {
			fmt.Printf("Website:      %s\n", record.Website)
		}
		if len(record.Authors) > 0 {
			fmt.Printf("Authors:      %s\n", strings.Join(record.Authors, ", "))
		}
		if record.License != "" {
			fmt.Printf("License:      %s\n", record.License)
		}
		if len(record.Distribution) > 0 {
			fmt.Printf("Distribution: %s\n", strings.Join(record.Distribution, ", "))
		}
		if len(record.Tags) > 0 {
			fmt.Printf("Tags:         %s\n", strings.Join(record.Tags, ", "))
		}
	},
}

func init() {
	agentInfoCmd.Flags().StringVar(&agentInfoSource, "source", string(agentdiscovery.SourceACPRegistry), "Discovery source: acp_registry, local, a2a_card, or a2a_catalog")
	agentInfoCmd.Flags().StringVar(&agentInfoCatalogURL, "catalog-url", "", "Catalog URL used when --source=a2a_catalog")
	agentCmd.AddCommand(agentInfoCmd)
}
