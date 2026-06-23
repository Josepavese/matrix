package main

import (
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/spf13/cobra"
)

var agentArgsCmd = &cobra.Command{
	Use:   "args",
	Short: "Manage agent argument overrides stored in SSOT",
}

var agentArgsListCmd = &cobra.Command{
	Use:   "list <agent_id>",
	Short: "List SSOT argument overrides appended to an agent command",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		agentID := args[0]
		ctx, cleanup, err := NewAgentContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		override, err := agentcfg.Load(ctx.Store, agentID)
		if err != nil {
			exitf("Error: %v", err)
		}
		for _, arg := range override.AppendArgs {
			fmt.Println(arg)
		}
	},
}

var agentArgsSetCmd = &cobra.Command{
	Use:   "set <agent_id> -- <arg> [arg...]",
	Short: "Replace appended SSOT argument overrides for an agent",
	Args:  cobra.MinimumNArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		agentID := args[0]
		appendArgs := filterAgentArgs(args[1:])
		if len(appendArgs) == 0 {
			exitf("Error: at least one argument is required")
		}
		ctx, cleanup, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		override, err := agentcfg.Load(ctx.Store, agentID)
		if err != nil {
			exitf("Error: %v", err)
		}
		override.AppendArgs = appendArgs
		if err := agentcfg.Save(ctx.Store, agentID, override); err != nil {
			exitf("Error: %v", err)
		}
		fmt.Printf("Agent %s append args replaced in SSOT override\n", agentID)
	},
}

var agentArgsAppendCmd = &cobra.Command{
	Use:   "append <agent_id> -- <arg> [arg...]",
	Short: "Append SSOT argument overrides to an agent command",
	Args:  cobra.MinimumNArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		agentID := args[0]
		appendArgs := filterAgentArgs(args[1:])
		if len(appendArgs) == 0 {
			exitf("Error: at least one argument is required")
		}
		ctx, cleanup, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		override, err := agentcfg.Load(ctx.Store, agentID)
		if err != nil {
			exitf("Error: %v", err)
		}
		override.AppendArgs = append(append([]string{}, override.AppendArgs...), appendArgs...)
		if err := agentcfg.Save(ctx.Store, agentID, override); err != nil {
			exitf("Error: %v", err)
		}
		fmt.Printf("Agent %s append args updated in SSOT override\n", agentID)
	},
}

var agentArgsClearCmd = &cobra.Command{
	Use:   "clear <agent_id>",
	Short: "Remove appended SSOT argument overrides for an agent",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		agentID := args[0]
		ctx, cleanup, err := NewAgentStoreContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		override, err := agentcfg.Load(ctx.Store, agentID)
		if err != nil {
			exitf("Error: %v", err)
		}
		override.AppendArgs = nil
		if err := agentcfg.Save(ctx.Store, agentID, override); err != nil {
			exitf("Error: %v", err)
		}
		fmt.Printf("Agent %s append args cleared from SSOT override\n", agentID)
	},
}

func filterAgentArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}

func init() {
	agentArgsCmd.AddCommand(agentArgsListCmd)
	agentArgsCmd.AddCommand(agentArgsSetCmd)
	agentArgsCmd.AddCommand(agentArgsAppendCmd)
	agentArgsCmd.AddCommand(agentArgsClearCmd)
	agentCmd.AddCommand(agentArgsCmd)
}
