package main

import (
	"encoding/json"
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/spf13/cobra"
)

var (
	agentSetEndpointKind            string
	agentSetEndpointTransport       string
	agentSetEndpointProtocolVersion string
	agentSetEndpointCardURL         string
)

var agentSetEndpointCmd = &cobra.Command{
	Use:   "set-endpoint <agent_id> <address>",
	Short: "Set a protocol-neutral remote endpoint for an agent",
	Args:  cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		agentID := args[0]
		address := args[1]

		ctx, cleanup, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		entry, err := agentcfg.LoadEntry(ctx.Store, agentID)
		if err != nil {
			exitf("Error: %v", err)
		}

		kind := agentSetEndpointKind
		if kind == "" {
			kind = string(middleware.ProtocolKindA2A)
		}
		transport := agentSetEndpointTransport
		if transport == "" {
			if kind == string(middleware.ProtocolKindA2A) {
				transport = "JSONRPC"
			} else {
				transport = "ws"
			}
		}

		entry.Config.Kind = kind
		entry.Config.Transport = transport
		entry.Config.Address = address
		entry.Config.ProtocolVersion = agentSetEndpointProtocolVersion
		entry.Config.CardURL = agentSetEndpointCardURL

		// Remote endpoints are not local binaries.
		entry.Config.Command = ""
		entry.Config.Args = nil
		entry.Config.EnvIsolation = false

		if err := agentcfg.SaveEntry(ctx.Store, agentID, entry); err != nil {
			exitf("Error: %v", err)
		}

		out, err := json.MarshalIndent(map[string]any{
			"agent_id":         agentID,
			"kind":             entry.Config.Kind,
			"transport":        entry.Config.Transport,
			"address":          entry.Config.Address,
			"card_url":         entry.Config.CardURL,
			"protocol_version": entry.Config.ProtocolVersion,
		}, "", "  ")
		if err != nil {
			exitf("Error: %v", err)
		}
		fmt.Println(string(out))
	},
}

func init() {
	agentSetEndpointCmd.Flags().StringVar(&agentSetEndpointKind, "kind", "a2a", "Protocol family: acp or a2a")
	agentSetEndpointCmd.Flags().StringVar(&agentSetEndpointTransport, "transport", "", "Endpoint transport/binding. Examples: ws, unix, JSONRPC, HTTP+JSON")
	agentSetEndpointCmd.Flags().StringVar(&agentSetEndpointProtocolVersion, "protocol-version", "", "Protocol version exposed by the endpoint")
	agentSetEndpointCmd.Flags().StringVar(&agentSetEndpointCardURL, "card-url", "", "Optional A2A agent card URL")
	agentCmd.AddCommand(agentSetEndpointCmd)
}
