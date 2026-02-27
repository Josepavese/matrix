package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Example: `  matrix config get provider.openai.key
  matrix config get provider.default`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mgr, cleanup, err := openConfigManager()
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		val, err := mgr.Get(args[0])
		if err != nil {
			exitf("Failed to get config: %v", err)
		}
		if val == "" {
			fmt.Fprintf(os.Stderr, "Key not found: %s\n", args[0])
			os.Exit(1)
		}
		fmt.Println(val)
	},
}

func init() {
	configCmd.AddCommand(configGetCmd)
}
