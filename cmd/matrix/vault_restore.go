package main

import (
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtimecheck"
	"github.com/Josepavese/matrix/internal/logic/vaultsec"
	"github.com/Josepavese/matrix/internal/providers/network"
	"github.com/Josepavese/matrix/internal/providers/osfs"
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
			cmd.Printf("restored %s (previous vault backed up to %s)\n", DefaultVaultPath, preBackup)
			return
		}
		cmd.Printf("restored %s\n", DefaultVaultPath)
	},
}

func init() {
	vaultRestoreCmd.Flags().StringVar(&vaultRestoreBackupDir, "backup-dir", "", "directory for the automatic pre-restore backup")
	vaultCmd.AddCommand(vaultRestoreCmd)
}
