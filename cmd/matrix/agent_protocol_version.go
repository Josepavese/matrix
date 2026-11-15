package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/pkg/zedacp"
	"github.com/spf13/cobra"
)

var agentProtocolVersionClear bool

var agentProtocolVersionCmd = &cobra.Command{
	Use:   "protocol-version <agent_id> <version>",
	Short: "Pin or clear an ACP agent's negotiated protocol generation",
	Long: `Pin the highest ACP generation a local agent may negotiate, or clear the
pin so it negotiates the highest generation Matrix supports. A version 1 pin is
the rollback for a peer whose version 2 payload is not conforming. Use --clear
to return to negotiation.`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(_ *cobra.Command, args []string) {
		agentID := args[0]
		ctx, cleanup, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault error: %v\n", err)
			os.Exit(1)
		}
		defer cleanup()
		entry, err := agentcfg.LoadEntry(ctx.Store, agentID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading entry: %v\n", err)
			os.Exit(1)
		}
		if entry.Config.Kind != "" && !strings.EqualFold(entry.Config.Kind, string("acp")) {
			fmt.Fprintf(os.Stderr, "Error: protocol-version pins apply to ACP agents only\n")
			os.Exit(1)
		}
		if agentProtocolVersionClear {
			if len(args) != 1 {
				fmt.Fprintf(os.Stderr, "Error: --clear takes no version argument\n")
				os.Exit(1)
			}
			entry.Config.ProtocolVersion = ""
		} else {
			if len(args) != 2 {
				fmt.Fprintf(os.Stderr, "Error: a version argument is required (or use --clear)\n")
				os.Exit(1)
			}
			version, err := zedacp.ConfiguredProtocolVersion(args[1])
			if err != nil || version == 0 {
				if err == nil {
					err = fmt.Errorf("a pin must be 1 or 2")
				}
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			entry.Config.ProtocolVersion = strconv.Itoa(version)
		}
		if err := agentcfg.SaveEntry(ctx.Store, agentID, entry); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving entry: %v\n", err)
			os.Exit(1)
		}
		if entry.Config.ProtocolVersion == "" {
			fmt.Printf("Cleared the ACP protocol pin for agent %s; initialize negotiates the highest supported generation.\n", agentID)
			return
		}
		fmt.Printf("Pinned agent %s to ACP protocol version %s.\n", agentID, entry.Config.ProtocolVersion)
	},
}

func init() {
	agentProtocolVersionCmd.Flags().BoolVar(&agentProtocolVersionClear, "clear", false, "Remove the pin and negotiate the highest supported generation")
	agentCmd.AddCommand(agentProtocolVersionCmd)
}
