package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Josepavese/matrix/internal/logic/agentcatalog"
	"github.com/Josepavese/matrix/internal/logic/agentdiscovery"
	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/bolt"
	networkprovider "github.com/Josepavese/matrix/internal/providers/network"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var (
	installA2AURL             string
	installA2ATransport       string
	installA2AProtocolVersion string
	installA2ACardURL         string
)

var installCmd = &cobra.Command{
	Use:   "install [agent_id]",
	Short: "Install an AI agent from the ACP Registry or register a remote A2A endpoint",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		agentID := args[0]
		netProv := networkprovider.NewProvider()

		// 1. Setup Dependencies
		provider, err := bolt.NewProvider(DefaultVaultPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault error: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = provider.Close() }()

		if installA2AURL != "" || installA2ACardURL != "" {
			if installA2AURL == "" {
				cardProvider, err := agentdiscovery.NewProvider(agentdiscovery.SourceA2ACard, agentdiscovery.Options{Net: netProv})
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error initializing A2A card discovery: %v\n", err)
					os.Exit(1)
				}
				record, err := cardProvider.Get(context.Background(), installA2ACardURL)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error resolving A2A card: %v\n", err)
					os.Exit(1)
				}
				installA2AURL = record.Address
				if installA2ATransport == "" {
					installA2ATransport = record.Transport
				}
				if installA2AProtocolVersion == "" {
					installA2AProtocolVersion = record.ProtocolVersion
				}
				installA2ACardURL = record.CardURL
			}
			if installA2AURL == "" {
				fmt.Fprintln(os.Stderr, "Error: unable to resolve an A2A endpoint address")
				os.Exit(1)
			}
			if err := agentcatalog.RegisterRemote(provider, agentcatalog.Entry{
				ID:              agentID,
				Name:            agentID,
				Source:          agentdiscovery.SourceA2ACard,
				Kind:            middleware.ProtocolKindA2A,
				Transport:       installA2ATransport,
				Address:         installA2AURL,
				CardURL:         installA2ACardURL,
				ProtocolVersion: installA2AProtocolVersion,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving A2A endpoint: %v\n", err)
				os.Exit(1)
			}
			transport := installA2ATransport
			if transport == "" {
				transport = "JSONRPC"
			}
			fmt.Printf("Successfully registered remote A2A agent '%s' at %s (%s)\n", agentID, installA2AURL, transport)
			return
		}

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
	installCmd.Flags().StringVar(&installA2AURL, "a2a-url", "", "Register the agent as a remote A2A endpoint instead of installing from the ACP registry")
	installCmd.Flags().StringVar(&installA2ATransport, "a2a-transport", "JSONRPC", "A2A transport binding for --a2a-url or an A2A card")
	installCmd.Flags().StringVar(&installA2AProtocolVersion, "a2a-protocol-version", "", "A2A protocol version for --a2a-url or an A2A card")
	installCmd.Flags().StringVar(&installA2ACardURL, "a2a-card-url", "", "A2A agent card URL or base URL used to discover a remote endpoint")
	rootCmd.AddCommand(installCmd)
}
