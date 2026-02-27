package main

import (
	"fmt"
	"os"

	"github.com/jose/matrix-v2/internal/logic/apm"
	execprovider "github.com/jose/matrix-v2/internal/providers/exec"
	"github.com/spf13/cobra"
)

var apmUpdateCmd = &cobra.Command{
	Use:   "update [package_name]",
	Short: "Update an AI agent package globally",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pkgName := args[0]
		mgr := apm.NewManager(execprovider.NewProvider())

		fmt.Printf("Orchestrating update of '%s' via APM...\n", pkgName)
		err := mgr.Update(pkgName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Package updated successfully.")
	},
}

func init() {
	apmCmd.AddCommand(apmUpdateCmd)
}
