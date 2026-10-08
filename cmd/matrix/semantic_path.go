package main

import (
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/semanticfs"
	"github.com/spf13/cobra"
)

var semanticPathCmd = &cobra.Command{
	Use: "path <agents|runs|workspaces> <id>", Short: "Encode the portable semantic path for a known native ID", Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "agents", "runs", "workspaces":
		default:
			return fmt.Errorf("unknown semantic namespace")
		}
		name, err := semanticfs.DirectoryName(args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), args[0]+"/"+name+"/status.json")
		return err
	},
}

func init() { semanticCmd.AddCommand(semanticPathCmd) }
