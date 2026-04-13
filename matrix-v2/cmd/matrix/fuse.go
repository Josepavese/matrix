package main

import "github.com/spf13/cobra"

var fuseCmd = &cobra.Command{
	Use:   "fuse",
	Short: "Experimental virtual filesystem surface",
}

func init() {
	rootCmd.AddCommand(fuseCmd)
}
