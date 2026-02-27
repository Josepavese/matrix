package main

import (
	"fmt"
	"os"

	"github.com/jose/matrix-v2/internal/logic/apm"
	execprovider "github.com/jose/matrix-v2/internal/providers/exec"
	"github.com/spf13/cobra"
)

var apmInstallCmd = &cobra.Command{
	Use:   "install [package_name]",
	Short: "Install an AI agent package globally",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pkgName := args[0]
		mgr := apm.NewManager(execprovider.NewProvider())

		fmt.Printf("Orchestrating installation of '%s' via APM...\n", pkgName)
		err := mgr.Install(pkgName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Package installed successfully.")
	},
}

func init() {
	apmCmd.AddCommand(apmInstallCmd)
}
