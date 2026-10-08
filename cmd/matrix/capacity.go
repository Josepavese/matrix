package main

import (
	"encoding/json"
	"fmt"

	"github.com/Josepavese/matrix/internal/providers/oscapacity"
	"github.com/spf13/cobra"
)

var capacityCmd = &cobra.Command{
	Use:   "capacity [workspace-directory]",
	Short: "Observe the actual workspace filesystem and runtime host capacity",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := resolveInvocationPath(args[0])
		if err != nil {
			return err
		}
		snapshot, observedErr := oscapacity.New().Observe(path)
		encoded, err := json.MarshalIndent(snapshot, "", "  ")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(encoded)); err != nil {
			return err
		}
		return observedErr
	},
}

func init() { rootCmd.AddCommand(capacityCmd) }
