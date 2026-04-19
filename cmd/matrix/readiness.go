package main

import (
	"os"

	"github.com/jose/matrix-v2/internal/logic/cmdutil"
	readinesslogic "github.com/jose/matrix-v2/internal/logic/readiness"
	"github.com/jose/matrix-v2/internal/logic/vaultsec"
	"github.com/jose/matrix-v2/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var readinessExpectRuntimeUp bool
var readinessStrict bool

var readinessCmd = &cobra.Command{
	Use:   "readiness",
	Short: "Evaluate whether Matrix meets the current local production-readiness baseline",
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
		storageReport, err := buildStorageDoctorReport()
		if err != nil {
			exitf("Storage doctor failed: %v", err)
		}
		vaultReport, err := vaultsec.BuildReport(osfs.NewFSProvider(), DefaultVaultPath)
		if err != nil {
			exitf("Vault doctor failed: %v", err)
		}

		report := readinesslogic.Evaluate(readinesslogic.Input{
			RuntimeReport:   runtimeReport,
			LoggingReport:   loggingReport,
			StorageReport:   storageReport,
			VaultReport:     vaultReport,
			ExpectRuntimeUp: readinessExpectRuntimeUp,
		})
		if err := cmdutil.PrintJSON(cmd, report); err != nil {
			exitf("failed to print readiness report: %v", err)
		}
		if report["status"] == "not_ready" || (readinessStrict && report["status"] != "ready") {
			os.Exit(2)
		}
	},
}

func init() {
	readinessCmd.Flags().BoolVar(&readinessExpectRuntimeUp, "expect-runtime-up", false, "treat an inactive local runtime as a readiness blocker")
	readinessCmd.Flags().BoolVar(&readinessStrict, "strict", false, "return non-zero unless readiness status is exactly ready")
	rootCmd.AddCommand(readinessCmd)
}
