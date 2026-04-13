package main

import (
	"github.com/jose/matrix-v2/internal/logic/cmdutil"
	"github.com/spf13/cobra"
)

var logsDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Inspect runtime logging health and retention state",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		report, err := buildLogsDoctorReport()
		if err != nil {
			exitf("Error: %v", err)
		}

		if err := cmdutil.PrintJSON(cmd, report); err != nil {
			exitf("Error: %v", err)
		}
	},
}

func init() {
	logsCmd.AddCommand(logsDoctorCmd)
}
