package main

import (
	"fmt"
	"os"

	"github.com/jose/matrix-v2/internal/logic/apm"
	execprovider "github.com/jose/matrix-v2/internal/providers/exec"
	"github.com/spf13/cobra"
)

var apmUninstallCmd = &cobra.Command{
	Use:   "uninstall [package_name]",
	Short: "Uninstall an AI agent package globally",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pkgName := args[0]
		mgr := apm.NewManager(execprovider.NewProvider())

		fmt.Printf("Orchestrating uninstallation of '%s' via APM...\n", pkgName)
		err := mgr.Uninstall(pkgName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Uninstallation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Package uninstalled successfully.")
	},
}

func init() {
	apmCmd.AddCommand(apmUninstallCmd)
}
