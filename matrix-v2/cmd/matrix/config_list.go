package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var configListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all configuration keys",
	Example: `  matrix config list`,
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		mgr, cleanup, err := openConfigManager()
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		keys, err := mgr.List()
		if err != nil {
			exitf("Failed to list config: %v", err)
		}
		if len(keys) == 0 {
			fmt.Println("(no configuration keys set)")
			return
		}
		for _, k := range keys {
			if strings.HasPrefix(k, "channel.") {
				continue
			}
			fmt.Println(k)
		}
	},
}

func init() {
	configCmd.AddCommand(configListCmd)
}
