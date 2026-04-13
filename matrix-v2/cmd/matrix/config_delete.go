package main

import (
	"github.com/spf13/cobra"
)

var configDeleteCmd = &cobra.Command{
	Use:     "delete <key>",
	Aliases: []string{"rm", "unset"},
	Short:   "Delete a configuration value",
	Example: `  matrix config delete provider.openai.key`,
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ensureConfigKeyAllowed(args[0])

		mgr, cleanup, err := openConfigManager()
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		if err := mgr.Delete(args[0]); err != nil {
			exitf("Failed to delete config: %v", err)
		}
		cmd.Printf("✓ deleted config.%s\n", args[0])
	},
}

func init() {
	configCmd.AddCommand(configDeleteCmd)
}
