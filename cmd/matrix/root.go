package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "matrix",
	Short: "Matrix is a local-first communication matrix for AI agents",
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		return configureMatrixHome()
	},
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Matrix")
	},
}

// Execute is the primary entrypoint for the CLI
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
