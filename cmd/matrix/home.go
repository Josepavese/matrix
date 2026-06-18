package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Josepavese/matrix/internal/logic/matrixhome"
	"github.com/spf13/cobra"
)

var activeMatrixHome string
var invocationCWD string

func configureMatrixHome() error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine invocation directory: %w", err)
	}
	invocationCWD = wd

	home, err := matrixhome.Configure()
	if err != nil {
		return err
	}
	activeMatrixHome = home
	return nil
}

func resolveInvocationPath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if invocationCWD == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to determine invocation directory: %w", err)
		}
		invocationCWD = wd
	}
	return filepath.Abs(filepath.Join(invocationCWD, path))
}

func resolveOptionalInvocationPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	return resolveInvocationPath(path)
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
