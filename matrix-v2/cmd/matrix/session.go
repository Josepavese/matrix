package main

import (
	"github.com/spf13/cobra"
)

// sessionCmd represents the parent session command
var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage ACP/A2A session mappings",
	Long: `Matrix Session Router (SSOT).
View or manage the mapping between physical channels (e.g., telegram_123) and logical Session IDs.`,
}

func init() {
	rootCmd.AddCommand(sessionCmd)
}
