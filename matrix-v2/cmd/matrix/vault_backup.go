package main

import (
	"time"

	"github.com/jose/matrix-v2/internal/logic/vaultsec"
	"github.com/jose/matrix-v2/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var vaultBackupDir string

var vaultBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a secure copy of the local vault",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		path, err := vaultsec.CreateBackup(osfs.NewFSProvider(), DefaultVaultPath, vaultBackupDir, time.Now())
		if err != nil {
			exitf("Vault backup failed: %v", err)
		}
		cmd.Println(path)
	},
}

func init() {
	vaultBackupCmd.Flags().StringVar(&vaultBackupDir, "dir", "", "backup destination directory")
	vaultCmd.AddCommand(vaultBackupCmd)
}
