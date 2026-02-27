package main

import (
	"fmt"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/apm"
	execprovider "github.com/jose/matrix-v2/internal/providers/exec"
	"github.com/spf13/cobra"
)

var apmListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all installed AI agent packages",
	Run: func(cmd *cobra.Command, args []string) {
		mgr := apm.NewManager(execprovider.NewProvider())

		packages := mgr.List()
		if len(packages) == 0 {
			fmt.Println("No packages currently installed in $PATH.")
			return
		}

		fmt.Println("Installed AI Packages:")
		fmt.Printf("- %s\n", strings.Join(packages, "\n- "))
	},
}

func init() {
	apmCmd.AddCommand(apmListCmd)
}
