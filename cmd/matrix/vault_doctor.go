package main

import (
	"encoding/json"

	"github.com/Josepavese/matrix/internal/logic/schema"
	"github.com/Josepavese/matrix/internal/logic/vaultsec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/Josepavese/matrix/internal/providers/runtimevault"
	"github.com/spf13/cobra"
)

var vaultDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Inspect vault security posture",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		provider, err := runtimevault.OpenReadOnly(DefaultVaultPath)
		if err != nil {
			exitf("Vault storage inspection failed: %v", err)
		}
		defer func() { _ = provider.Close() }()
		securityReport, err := vaultsec.BuildReport(osfs.NewFSProvider(), DefaultVaultPath, provider)
		if err != nil {
			exitf("Vault doctor failed: %v", err)
		}
		report := map[string]any{"security": securityReport}
		if schemaReport, err := schema.LoadReport(provider); err == nil {
			report["schema"] = schemaReport
		}
		blob, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			exitf("Error: %v", err)
		}
		cmd.Println(string(blob))
	},
}

func init() {
	vaultCmd.AddCommand(vaultDoctorCmd)
}
