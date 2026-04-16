package main

import (
	"github.com/jose/matrix-v2/internal/logic/matrixhome"
	"github.com/spf13/cobra"
)

var activeMatrixHome string

func configureMatrixHome() error {
	home, err := matrixhome.Configure()
	if err != nil {
		return err
	}
	activeMatrixHome = home
	return nil
}

var homeCmd = &cobra.Command{
	Use:   "home",
	Short: "Print the resolved Matrix PAL home",
	Run: func(cmd *cobra.Command, _ []string) {
		cmd.Println(activeMatrixHome)
	},
}

func init() {
	rootCmd.AddCommand(homeCmd)
}
