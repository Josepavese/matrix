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

var agentSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search the ACP Registry for available agents",
	Long:  "Search the ACP Registry for agents by name, description, or ID. Without a query, lists all agents.",
	Args:  cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		query := ""
		if len(args) == 1 {
			query = strings.ToLower(args[0])
		}

		provider, err := bolt.NewReadOnlyProvider(DefaultVaultPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault error: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = provider.Close() }()

		netProv := networkprovider.NewProvider()
		regClient := agentmgr.NewCachingRegistryClient(netProv, "", provider)

		ctx := context.Background()
		index, err := regClient.FetchIndexCached(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to fetch registry: %v\n", err)
			os.Exit(1)
		}

		matched := 0
		for _, agent := range index.Agents {
			if query != "" && !matchesAgent(agent, query) {
				continue
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", agent.ID, agent.Name, agent.Version, strings.Join(agent.DistTypes(), ","))
			matched++
		}

		if matched == 0 && query != "" {
			fmt.Fprintf(os.Stderr, "No agents matching '%s' found.\n", query)
		}
	},
}

func matchesAgent(a agentmgr.AgentManifest, query string) bool {
	if strings.Contains(strings.ToLower(a.ID), query) {
		return true
	}
	if strings.Contains(strings.ToLower(a.Name), query) {
		return true
	}
	if strings.Contains(strings.ToLower(a.Description), query) {
		return true
	}
	return false
}

func init() {
	agentCmd.AddCommand(agentSearchCmd)
}
