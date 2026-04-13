package main

import (
	logiclogging "github.com/jose/matrix-v2/internal/logic/logging"
	"github.com/jose/matrix-v2/internal/providers/osfs"
	"github.com/spf13/cobra"
)

var logsTailLines int
var logsTailFollow bool

var logsTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Print the last lines of the active runtime log file",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, cleanup, err := openLogConfig()
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		fsProv := osfs.NewFSProvider()
		lines, err := logiclogging.TailFile(fsProv, cfg.FilePath, logsTailLines)
		if err != nil {
			exitf("Failed to read log file: %v", err)
		}

		for _, line := range lines {
			cmd.Println(line)
		}

		if !logsTailFollow {
			return
		}

		info, err := fsProv.Stat(cfg.FilePath)
		if err != nil {
			exitf("Failed to stat log file: %v", err)
		}

		if err := logiclogging.FollowFile(fsProv, cfg.FilePath, info.Size(), func(line string) {
			cmd.Println(line)
		}); err != nil {
			exitf("Failed to follow log file: %v", err)
		}
	},
}

func init() {
	logsTailCmd.Flags().IntVarP(&logsTailLines, "lines", "n", 20, "number of lines to print")
	logsTailCmd.Flags().BoolVarP(&logsTailFollow, "follow", "f", false, "follow the log file for appended lines")
	logsCmd.AddCommand(logsTailCmd)
}
