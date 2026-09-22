package main

import (
	"fmt"
	"os"

	"github.com/Josepavese/matrix/internal/logic/cmdutil"
	readinesslogic "github.com/Josepavese/matrix/internal/logic/readiness"
	"github.com/Josepavese/matrix/internal/logic/vaultsec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/Josepavese/matrix/internal/providers/runtimevault"
	"github.com/spf13/cobra"
)

var readinessExpectRuntimeUp bool
var readinessStrict bool

var readinessCmd = &cobra.Command{
	Use:   "readiness",
	Short: "Evaluate whether Matrix meets the current local production-readiness baseline",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		report, err := buildReadinessReport(readinessExpectRuntimeUp)
		if err != nil {
			exitf("%v", err)
		}
		if err := cmdutil.PrintJSON(cmd, report); err != nil {
			exitf("failed to print readiness report: %v", err)
		}
		if code := readinessExitCode(report["status"], readinessStrict); code != 0 {
			os.Exit(code)
		}
	},
}

// buildReadinessReport assembles every local signal into one readiness result.
//
// A vault that cannot be inspected is reported as a blocker rather than as a
// command failure: on a machine that has never run Matrix the vault does not
// exist yet, and exiting here would hide the rest of the report exactly when an
// operator needs it most.
func buildReadinessReport(expectRuntimeUp bool) (map[string]any, error) {
	runtimeReport, err := buildRuntimeDoctorReport()
	if err != nil {
		return nil, fmt.Errorf("runtime doctor failed: %w", err)
	}
	loggingReport, err := buildLogsDoctorReport()
	if err != nil {
		return nil, fmt.Errorf("logging doctor failed: %w", err)
	}
	storageReport, err := buildStorageDoctorReport()
	if err != nil {
		return nil, fmt.Errorf("storage doctor failed: %w", err)
	}

	var vaultReport map[string]any
	vaultStore, vaultErr := runtimevault.OpenReadOnly(DefaultVaultPath)
	if vaultErr != nil {
		fmt.Fprintf(os.Stderr, "warning: vault was not inspected: %v\n", vaultErr)
	} else {
		defer func() { _ = vaultStore.Close() }()
		vaultReport, err = vaultsec.BuildReport(osfs.NewFSProvider(), DefaultVaultPath, vaultStore)
		if err != nil {
			return nil, fmt.Errorf("vault doctor failed: %w", err)
		}
	}

	return readinesslogic.Evaluate(readinesslogic.Input{
		RuntimeReport:   runtimeReport,
		LoggingReport:   loggingReport,
		StorageReport:   storageReport,
		VaultReport:     vaultReport,
		ExpectRuntimeUp: expectRuntimeUp,
	}), nil
}

func readinessExitCode(status any, strict bool) int {
	if strict && status != "ready" {
		return 2
	}
	return 0
}

func init() {
	readinessCmd.Flags().BoolVar(&readinessExpectRuntimeUp, "expect-runtime-up", false, "treat an inactive local runtime as a readiness blocker")
	readinessCmd.Flags().BoolVar(&readinessStrict, "strict", false, "return non-zero unless readiness status is exactly ready")
	rootCmd.AddCommand(readinessCmd)
}
