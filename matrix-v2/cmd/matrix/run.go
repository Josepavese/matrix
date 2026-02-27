package main

import (
	"fmt"
	"os"

	"github.com/jose/matrix-v2/internal/logic/daemon"
	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	networkprovider "github.com/jose/matrix-v2/internal/providers/network"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the Matrix V2 background daemon",
	Run: func(cmd *cobra.Command, args []string) {
		provider, err := bolt.NewProvider("matrix-vault.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Vault initialization error: %v\n", err)
			os.Exit(1)
		}
		defer provider.Close()

		v := vault.NewVault(provider)
		netProvider := networkprovider.NewProvider()
		srv := daemon.NewServer(v, netProvider)

		fmt.Println("Starting Matrix Daemon on :9090...")
		if err := srv.Start(":9090"); err != nil {
			fmt.Fprintf(os.Stderr, "Daemon error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
