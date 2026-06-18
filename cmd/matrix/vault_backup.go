package main

import (
	"time"

	"github.com/Josepavese/matrix/internal/logic/vaultsec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var vaultBackupDir string

var vaultBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a secure copy of the local vault",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		backupDir, err := resolveOptionalInvocationPath(vaultBackupDir)
		if err != nil {
			exitf("Vault backup failed: invalid backup directory: %v", err)
		}
		path, err := vaultsec.CreateBackup(osfs.NewFSProvider(), DefaultVaultPath, backupDir, time.Now())
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
