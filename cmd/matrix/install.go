package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcatalog"
	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentstore"
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
	installAllowUnverified    bool
	installStoreRoot          string
	installStoreDigest        string
)

var installCmd = &cobra.Command{
	Use:   "install [agent_id]",
	Short: "Install an AI agent from the ACP Registry or register a remote A2A endpoint",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		agentID := args[0]
		netProv := networkprovider.NewProvider()
		if strings.TrimSpace(installStoreRoot) != "" && (installA2AURL != "" || installA2ACardURL != "") {
			fmt.Fprintln(os.Stderr, "Error: --store cannot be combined with --a2a-url/--a2a-card-url: a remote A2A endpoint runs no local software")
			os.Exit(1)
		}

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
		storeRoot := strings.TrimSpace(installStoreRoot)
		if storeRoot == "" {
			storeRoot = strings.TrimSpace(os.Getenv(agentstore.EnvName))
		}
		if storeRoot != "" {
			installFromSharedStore(ctx, agentID, storeRoot)
			return
		}
		if installStoreDigest != "" {
			fmt.Fprintf(os.Stderr, "Error: --digest requires --store (or %s); a digest alone does not name a store\n", agentstore.EnvName)
			os.Exit(1)
		}
		ctx.Installer.SetAllowUnverified(installAllowUnverified)
		if err := ctx.Installer.Install(context.Background(), agentID); err != nil {
			fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully installed and registered agent '%s'\n", agentID)
	},
}

// installFromSharedStore registers an agent whose software is already a verified
// generation of the shared immutable store. The store is read-only for this
// runtime, so the bytes are not downloaded again and no per-home copy is made;
// the agent record points at the digest-addressed generation.
func installFromSharedStore(ctx *InstallerContext, agentID, storeRoot string) {
	digest := strings.TrimSpace(installStoreDigest)
	if digest == "" {
		resolved, err := ctx.Installer.ResolveStoreGeneration(context.Background(), agentID, storeRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Store install failed: %v\n", err)
			os.Exit(1)
		}
		digest = resolved.Digest
	}
	if err := ctx.Installer.InstallFromStore(context.Background(), agentID, storeRoot, digest); err != nil {
		fmt.Fprintf(os.Stderr, "Store install failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully installed and registered agent '%s' from store %s generation %s\n", agentID, storeRoot, digest)
}

func init() {
	installCmd.Flags().StringVar(&installA2AURL, "a2a-url", "", "Register the agent as a remote A2A endpoint instead of installing from the ACP registry")
	installCmd.Flags().StringVar(&installA2ATransport, "a2a-transport", "JSONRPC", "A2A transport binding for --a2a-url or an A2A card")
	installCmd.Flags().StringVar(&installA2AProtocolVersion, "a2a-protocol-version", "", "A2A protocol version for --a2a-url or an A2A card")
	installCmd.Flags().StringVar(&installA2ACardURL, "a2a-card-url", "", "A2A agent card URL or base URL used to discover a remote endpoint")
	installCmd.Flags().StringVar(&installA2ATenant, "a2a-tenant", "", "Optional A2A tenant for direct endpoint registration")
	installCmd.Flags().StringArrayVar(&installA2AHeaders, "a2a-header", nil, "Governed A2A header as Name=Value (repeatable; stored only in the Vault)")
	installCmd.Flags().BoolVar(&installAllowUnverified, "allow-unverified", false, "Install a binary distribution whose registry entry publishes no sha256 (recorded in the install evidence; refused by default)")
	installCmd.Flags().StringVar(&installStoreRoot, "store", "", "Install the agent from a verified generation of a shared immutable artifact store instead of a per-home copy (defaults to $"+agentstore.EnvName+")")
	installCmd.Flags().StringVar(&installStoreDigest, "digest", "", "Exact store generation to install with --store; resolved from the current registry version when omitted")
	rootCmd.AddCommand(installCmd)
}
