package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install [package]",
	Short: "Install an AI Package via APM",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Matrix APM install invoked")
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
