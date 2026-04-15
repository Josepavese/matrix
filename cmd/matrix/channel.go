package main

import (
	"io"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/channelcfg"
	"github.com/jose/matrix-v2/internal/logic/cmdutil"
	"github.com/jose/matrix-v2/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var channelCmd = &cobra.Command{Use: "channel", Short: "Manage channel provider configuration in the SSOT vault"}
var channelSetStdin bool

var channelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List supported channel providers",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		if err := cmdutil.PrintJSON(cmd, map[string]any{"providers": channelcfg.SupportedProviders()}); err != nil {
			exitf("Error: %v", err)
		}
	},
}

var channelShowCmd = &cobra.Command{
	Use:   "show <provider>",
	Short: "Show effective and override configuration for a channel provider",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfgMgr, cleanup, err := openConfigManager()
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		state, err := channelcfg.DescribeProvider(osfs.NewConfigProvider(), cfgMgr, args[0])
		if err != nil {
			exitf("Error: %v", err)
		}
		if err := cmdutil.PrintJSON(cmd, state); err != nil {
			exitf("Error: %v", err)
		}
	},
}

var channelSetCmd = &cobra.Command{
	Use:   "set <provider> <key> <value>",
	Short: "Set a channel override in the SSOT vault",
	Args: func(cmd *cobra.Command, args []string) error {
		if channelSetStdin {
			return cobra.ExactArgs(2)(cmd, args)
		}
		return cobra.ExactArgs(3)(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		cfgMgr, cleanup, err := openConfigManager()
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		value := ""
		if channelSetStdin {
			data, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				exitf("Error: %v", err)
			}
			value = strings.TrimRight(string(data), "\r\n")
		} else {
			value = args[2]
		}

		if err := channelcfg.SetOverride(cfgMgr, args[0], args[1], value); err != nil {
			exitf("Error: %v", err)
		}
		if redacted, ok := channelcfg.RedactValue(args[1], value); ok {
			value = redacted
		}
		cmd.Printf("✓ channel.%s.%s = %s\n", args[0], args[1], value)
	},
}

var channelDeleteCmd = &cobra.Command{
	Use:   "delete <provider> <key>",
	Short: "Delete a channel override from the SSOT vault",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		cfgMgr, cleanup, err := openConfigManager()
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()
		if err := channelcfg.DeleteOverride(cfgMgr, args[0], args[1]); err != nil {
			exitf("Error: %v", err)
		}
		cmd.Printf("✓ deleted channel.%s.%s\n", args[0], args[1])
	},
}

func init() {
	channelSetCmd.Flags().BoolVar(&channelSetStdin, "stdin", false, "read the value from stdin instead of argv")
	channelCmd.AddCommand(channelListCmd, channelShowCmd, channelSetCmd, channelDeleteCmd)
	rootCmd.AddCommand(channelCmd)
}
