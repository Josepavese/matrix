package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print Matrix version information",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("matrix %s\ncommit %s\nbuilt %s\n", version, commit, date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
