package main

import "github.com/spf13/cobra"

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Inspect Matrix runtime logging",
}

func init() {
	rootCmd.AddCommand(logsCmd)
}
