package main

import (
	"github.com/spf13/cobra"
)

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Example: `  matrix config set provider.openai.key sk-abc123
  matrix config set provider.default openai`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		mgr, cleanup, err := openConfigManager()
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		if err := mgr.Set(args[0], args[1]); err != nil {
			exitf("Failed to set config: %v", err)
		}
		cmd.Printf("✓ config.%s = %s\n", args[0], args[1])
	},
}

func init() {
	configCmd.AddCommand(configSetCmd)
}
