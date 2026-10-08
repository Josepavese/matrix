package main

import (
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"
)

var semanticSummaries bool

var semanticCmd = &cobra.Command{Use: "fs", Short: "Read selected Matrix state through the semantic filesystem"}

var semanticListCmd = &cobra.Command{
	Use: "list [path]", Short: "List a read-only semantic directory", Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		view, closeFn, err := openSemanticFS(semanticSummaries)
		if err != nil {
			return err
		}
		defer closeFn()
		name := "."
		if len(args) > 0 {
			name = args[0]
		}
		entries, err := fs.ReadDir(view, name)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), entry.Name()); err != nil {
				return err
			}
		}
		return nil
	},
}

var semanticReadCmd = &cobra.Command{
	Use: "read <path>", Short: "Read one semantic snapshot; summaries require opt-in", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		view, closeFn, err := openSemanticFS(semanticSummaries)
		if err != nil {
			return err
		}
		defer closeFn()
		data, err := fs.ReadFile(view, args[0])
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(data)
		return err
	},
}

func init() {
	semanticCmd.PersistentFlags().BoolVar(&semanticSummaries, "include-summaries", false, "expose terminal summaries; may contain private conversation content")
	semanticCmd.AddCommand(semanticListCmd, semanticReadCmd)
	rootCmd.AddCommand(semanticCmd)
}
