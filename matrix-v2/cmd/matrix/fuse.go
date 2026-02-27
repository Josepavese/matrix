package main

import "github.com/spf13/cobra"

var fuseCmd = &cobra.Command{
	Use:   "fuse",
	Short: "Manage Matrix Virtual Filesystem",
}

func init() {
	rootCmd.AddCommand(fuseCmd)
}
