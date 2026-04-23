package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentdiscovery"
	networkprovider "github.com/Josepavese/matrix/internal/providers/network"
	"github.com/spf13/cobra"
)

var (
	agentSearchInstalled  bool
	agentSearchSource     string
	agentSearchCatalogURL string
)

var agentSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search agent discovery sources",
	Long:  "Search the local SSOT, the ACP registry, or an A2A catalog through a unified discovery layer.",
	Args:  cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		query := ""
		if len(args) == 1 {
			query = strings.ToLower(args[0])
		}

		source := agentdiscovery.Source(agentSearchSource)
		if agentSearchInstalled {
			source = agentdiscovery.SourceLocal
		}

		records, err := searchAgents(source, query, agentSearchCatalogURL)
		if err != nil {
			exitf("Error: %v", err)
		}
		for _, record := range records {
			extra := record.Address
			if source == agentdiscovery.SourceACPRegistry {
				extra = strings.Join(record.Distribution, ",")
			}
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n", record.ID, record.Kind, record.Transport, record.Version, extra)
		}
		if len(records) == 0 && query != "" {
			fmt.Printf("No agents matching '%s' found in source '%s'.\n", query, source)
		}
	},
}

func init() {
	agentSearchCmd.Flags().BoolVar(&agentSearchInstalled, "installed", false, "Search agents already registered in the local SSOT instead of the remote ACP registry")
	agentSearchCmd.Flags().StringVar(&agentSearchSource, "source", string(agentdiscovery.SourceACPRegistry), "Discovery source: acp_registry, local, or a2a_catalog")
	agentSearchCmd.Flags().StringVar(&agentSearchCatalogURL, "catalog-url", "", "Catalog URL used when --source=a2a_catalog")
	agentCmd.AddCommand(agentSearchCmd)
}

func searchAgents(source agentdiscovery.Source, query string, catalogURL string) ([]agentdiscovery.Record, error) {
	ctx := context.Background()
	opts := agentdiscovery.Options{
		Net:        networkprovider.NewProvider(),
		CatalogURL: catalogURL,
	}

	if source == agentdiscovery.SourceLocal || source == agentdiscovery.SourceACPRegistry {
		agentCtx, cleanup, err := NewAgentContext(DefaultVaultPath)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		opts.Storage = agentCtx.Store
		opts.Registry = agentCtx.Registry
	}

	provider, err := agentdiscovery.NewProvider(source, opts)
	if err != nil {
		return nil, err
	}
	return provider.Search(ctx, query)
}
