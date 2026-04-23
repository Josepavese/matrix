package main

import (
	"encoding/json"

	"github.com/Josepavese/matrix/internal/logic/schema"
	"github.com/Josepavese/matrix/internal/logic/vaultsec"
	"github.com/Josepavese/matrix/internal/providers/bolt"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var vaultDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Inspect vault security posture",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		securityReport, err := vaultsec.BuildReport(osfs.NewFSProvider(), DefaultVaultPath)
		if err != nil {
			exitf("Vault doctor failed: %v", err)
		}
		report := map[string]any{
			"security": securityReport,
		}
		if provider, err := bolt.NewReadOnlyProvider(DefaultVaultPath); err == nil {
			defer func() { _ = provider.Close() }()
			if schemaReport, err := schema.LoadReport(provider); err == nil {
				report["schema"] = schemaReport
			}
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
