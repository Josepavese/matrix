package main

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

var logsShowConfigCmd = &cobra.Command{
	Use:   "show-config",
	Short: "Show effective logging configuration",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		cfg, cleanup, err := openLogConfig()
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		out := map[string]any{
			"level":       cfg.Level.String(),
			"format":      cfg.Format,
			"sink":        cfg.Sink,
			"file_path":   cfg.FilePath,
			"max_bytes":   cfg.MaxBytes,
			"max_backups": cfg.MaxBackups,
			"stderr":      cfg.StdErr,
			"acp_wire":    cfg.ACPWire,
		}

		blob, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			exitf("Error: %v", err)
		}
		cmd.Println(string(blob))
	},
}

func init() {
	logsCmd.AddCommand(logsShowConfigCmd)
}
