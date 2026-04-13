package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "matrix",
	Short: "Matrix V2 is a system daemon and CLI for AI Orchestration",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Matrix V2")
	},
}

// Execute is the primary entrypoint for the CLI
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
