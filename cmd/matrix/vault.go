package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Manage Matrix V2 SSOT Vault",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Matrix Vault Options: set, get, backup, restore, doctor, migrate, seal")
	},
}

func init() {
	rootCmd.AddCommand(vaultCmd)
}
