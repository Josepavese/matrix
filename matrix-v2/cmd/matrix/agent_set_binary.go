package main

import (
	"fmt"
	"os"

	"github.com/jose/matrix-v2/internal/logic/agentcfg"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	"github.com/spf13/cobra"
)

var (
	setBinaryProtocol string
	setBinaryArgs     []string
)

var agentSetBinaryCmd = &cobra.Command{
	Use:   "set-binary <agent_id> <path>",
	Short: "Manually point an agent ID to an existing binary on the system",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		agentID := args[0]
		binaryPath := args[1]

		// 1. Verify path exists
		if _, err := os.Stat(binaryPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: binary path does not exist: %v\n", err)
			os.Exit(1)
		}

		// 2. Setup Vault
		provider, err := bolt.NewProvider(DefaultVaultPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault error: %v\n", err)
			os.Exit(1)
		}
		defer provider.Close()

		// 3. Load or Create Entry
		entry, err := agentcfg.LoadEntry(provider, agentID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading entry: %v\n", err)
			os.Exit(1)
		}

		// Update path
		entry.Config.Command = binaryPath

		// Apply --protocol flag if provided
		if setBinaryProtocol != "" {
			entry.Config.Protocol = setBinaryProtocol
		} else if entry.Config.Protocol == "" {
			entry.Config.Protocol = "stdio"
		}

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
		if err := agentcfg.SaveEntry(provider, agentID, entry); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving entry: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully mapped agent %s to %s (protocol=%s, args=%v)\n", agentID, binaryPath, entry.Config.Protocol, entry.Config.Args)
	},
}

func init() {
	agentSetBinaryCmd.Flags().StringVar(&setBinaryProtocol, "protocol", "", "Transport protocol (stdio, ws, unix)")
	agentSetBinaryCmd.Flags().StringArrayVar(&setBinaryArgs, "args", nil, "Arguments for the agent binary")
	agentCmd.AddCommand(agentSetBinaryCmd)
}
