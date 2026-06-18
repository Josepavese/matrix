package main

import (
	"fmt"
	"os"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/spf13/cobra"
)

var (
	setBinaryKind            string
	setBinaryTransport       string
	setBinaryProtocolVersion string
	setBinaryArgs            []string
)

var agentSetBinaryCmd = &cobra.Command{
	Use:   "set-binary <agent_id> <path>",
	Short: "Manually point an agent ID to an existing binary on the system",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		agentID := args[0]
		binaryPath, err := resolveInvocationPath(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid binary path: %v\n", err)
			os.Exit(1)
		}

		// 1. Verify path exists
		if _, err := os.Stat(binaryPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: binary path does not exist: %v\n", err)
			os.Exit(1)
		}

		// 2. Setup Vault
		ctx, cleanup, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault error: %v\n", err)
			os.Exit(1)
		}
		defer cleanup()

		// 3. Load or Create Entry
		entry, err := agentcfg.LoadEntry(ctx.Store, agentID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading entry: %v\n", err)
			os.Exit(1)
		}

		// Update path
		entry.Config.Command = binaryPath

		// Apply --kind flag if provided.
		if setBinaryKind != "" {
			entry.Config.Kind = setBinaryKind
		}
		if entry.Config.Kind == "" {
			entry.Config.Kind = "acp"
		}
		if setBinaryTransport != "" {
			entry.Config.Transport = setBinaryTransport
		} else if entry.Config.Transport == "" {
			entry.Config.Transport = "stdio"
		}
		if setBinaryProtocolVersion != "" {
			entry.Config.ProtocolVersion = setBinaryProtocolVersion
		}
		entry.Config.Address = ""
		entry.Config.CardURL = ""

		// Apply --args flag if provided
		if cmd.Flags().Changed("args") {
			filtered := make([]string, 0, len(setBinaryArgs))
			for _, a := range setBinaryArgs {
				if a != "" {
					filtered = append(filtered, a)
				}
			}
			entry.Config.Args = filtered
		} else if entry.Config.Args == nil {
			entry.Config.Args = []string{}
		}

		// 4. Save
		if err := agentcfg.SaveEntry(ctx.Store, agentID, entry); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving entry: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully mapped agent %s to %s (kind=%s, transport=%s, args=%v)\n", agentID, binaryPath, entry.Config.Kind, entry.Config.Transport, entry.Config.Args)
	},
}

func init() {
	agentSetBinaryCmd.Flags().StringVar(&setBinaryKind, "kind", "", "Protocol family (acp or a2a). Defaults to acp for local binaries")
	agentSetBinaryCmd.Flags().StringVar(&setBinaryTransport, "transport", "stdio", "Transport used by the binary endpoint")
	agentSetBinaryCmd.Flags().StringVar(&setBinaryProtocolVersion, "protocol-version", "", "Optional protocol version exposed by the agent")
	agentSetBinaryCmd.Flags().StringArrayVar(&setBinaryArgs, "args", nil, "Arguments for the agent binary")
	agentCmd.AddCommand(agentSetBinaryCmd)
}
