package main

import (
	"github.com/jose/matrix-v2/internal/logic/cmdutil"
	"github.com/jose/matrix-v2/internal/logic/orchestration"
	"github.com/spf13/cobra"
)

var (
	orchestrationCmd = &cobra.Command{
		Use:   "orchestration",
		Short: "Inspect Matrix orchestration capabilities",
	}

	orchestrationCapabilitiesCmd = &cobra.Command{
		Use:   "capabilities",
		Short: "Print the machine-readable orchestration profile",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := cmdutil.PrintJSON(cmd, orchestration.ProfileV1()); err != nil {
				exitf("failed to print orchestration profile: %v", err)
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(orchestrationCmd)
	orchestrationCmd.AddCommand(orchestrationCapabilitiesCmd)
}
