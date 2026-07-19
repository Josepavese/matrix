package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Josepavese/matrix/internal/logic/agentcatalog"
	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	networkprovider "github.com/Josepavese/matrix/internal/providers/network"
	"github.com/spf13/cobra"
)

var (
	installA2AURL             string
	installA2ATransport       string
	installA2AProtocolVersion string
	installA2ACardURL         string
	installA2ATenant          string
	installA2AHeaders         []string
)

var installCmd = &cobra.Command{
	Use:   "install [agent_id]",
	Short: "Install an AI agent from the ACP Registry or register a remote A2A endpoint",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		agentID := args[0]
		netProv := networkprovider.NewProvider()

		// 1. Setup Dependencies
		ctx, cleanup, err := NewInstallerContext(DefaultVaultPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault error: %v\n", err)
			os.Exit(1)
		}
		defer cleanup()

		if installA2AURL != "" || installA2ACardURL != "" {
			headers, err := agentcfg.ParseHeaders(installA2AHeaders)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			entry, err := agentcatalog.ResolveAndRegisterA2A(context.Background(), ctx.Store, netProv, agentcatalog.A2ARegistration{
				ID: agentID, Address: installA2AURL, Transport: installA2ATransport,
				ProtocolVersion: installA2AProtocolVersion, CardURL: installA2ACardURL,
				Tenant: installA2ATenant, Headers: headers,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error saving A2A endpoint: %v\n", err)
				os.Exit(1)
			}
			transport := entry.Transport
			if transport == "" {
				transport = "JSONRPC"
			}
			fmt.Printf("Successfully registered remote A2A agent '%s' at %s (%s)\n", agentID, entry.Address, transport)
			return
		}

		// 2. Execute Install
		if err := ctx.Installer.Install(context.Background(), agentID); err != nil {
			fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully installed and registered agent '%s'\n", agentID)
	},
}

func init() {
	installCmd.Flags().StringVar(&installA2AURL, "a2a-url", "", "Register the agent as a remote A2A endpoint instead of installing from the ACP registry")
	installCmd.Flags().StringVar(&installA2ATransport, "a2a-transport", "JSONRPC", "A2A transport binding for --a2a-url or an A2A card")
	installCmd.Flags().StringVar(&installA2AProtocolVersion, "a2a-protocol-version", "", "A2A protocol version for --a2a-url or an A2A card")
	installCmd.Flags().StringVar(&installA2ACardURL, "a2a-card-url", "", "A2A agent card URL or base URL used to discover a remote endpoint")
	installCmd.Flags().StringVar(&installA2ATenant, "a2a-tenant", "", "Optional A2A tenant for direct endpoint registration")
	installCmd.Flags().StringArrayVar(&installA2AHeaders, "a2a-header", nil, "Governed A2A header as Name=Value (repeatable; stored only in the Vault)")
	rootCmd.AddCommand(installCmd)
}
