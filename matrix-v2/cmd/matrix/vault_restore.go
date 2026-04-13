package main

import (
	"time"

	"github.com/jose/matrix-v2/internal/logic/runtimecheck"
	"github.com/jose/matrix-v2/internal/logic/vaultsec"
	"github.com/jose/matrix-v2/internal/providers/network"
	"github.com/jose/matrix-v2/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var vaultRestoreBackupDir string

var vaultRestoreCmd = &cobra.Command{
	Use:   "restore <backup_path>",
	Short: "Restore the local vault from a backup file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		netProv := network.NewProvider()
		if runtimecheck.CanDial(netProv, "127.0.0.1:9090") || runtimecheck.CanDial(netProv, "127.0.0.1:9091") {
			exitf("Refusing to restore while Matrix runtime is active on 127.0.0.1:9090/9091")
		}
		fsProv := osfs.NewFSProvider()
		preBackup, err := vaultsec.RestoreBackup(fsProv, args[0], DefaultVaultPath, vaultRestoreBackupDir, time.Now())
		if err != nil {
			exitf("Vault restore failed: %v", err)
		}
		if preBackup != "" {
			cmd.Printf("restored matrix-vault.db (previous vault backed up to %s)\n", preBackup)
			return
		}
		cmd.Println("restored matrix-vault.db")
	},
}

func init() {
	vaultRestoreCmd.Flags().StringVar(&vaultRestoreBackupDir, "backup-dir", "", "directory for the automatic pre-restore backup")
	vaultCmd.AddCommand(vaultRestoreCmd)
}
