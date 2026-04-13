package main

import (
	"github.com/jose/matrix-v2/internal/logic/cmdutil"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Inspect Matrix runtime health",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		runtimeReport, err := buildRuntimeDoctorReport()
		if err != nil {
			exitf("Runtime doctor failed: %v", err)
		}
		loggingReport, err := buildLogsDoctorReport()
		if err != nil {
			exitf("Logging doctor failed: %v", err)
		}

		report := map[string]any{
			"runtime": runtimeReport,
			"logging": loggingReport,
		}

		if err := cmdutil.PrintJSON(cmd, report); err != nil {
			exitf("Error: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
