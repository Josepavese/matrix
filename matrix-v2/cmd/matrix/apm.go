package main

import "github.com/spf13/cobra"

var apmCmd = &cobra.Command{
	Use:   "apm",
	Short: "Matrix AI Package Manager",
}

func init() {
	rootCmd.AddCommand(apmCmd)
}
