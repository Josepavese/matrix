package main

import (
	"github.com/spf13/cobra"
)

var vaultSummaryMaxBytes int

var vaultSummaryCmd = &cobra.Command{
	Use:   "summary [run id]",
	Short: "Print only the terminal summary of a run",
	Long: `Print only the terminal summary of a run record, and nothing else.

This is the bounded, final-only getter for the content the record references as
its summary: no events, no transcript, no diagnostic fields. Over the cap the
command refuses with ERR_VAULT_SUMMARY_TOO_LARGE instead of truncating, because
a truncated summary is a wrong answer that looks right.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runVaultSummary(cmd, args[0]); err != nil {
			exitVaultReadError(err)
		}
	},
}

func init() {
	vaultSummaryCmd.Flags().IntVar(&vaultSummaryMaxBytes, "max-bytes", defaultVaultGetMaxBytes, "refuse a summary larger than this many bytes")
	vaultCmd.AddCommand(vaultSummaryCmd)
}

func runVaultSummary(cmd *cobra.Command, idOrKey string) error {
	blob, err := readVaultRecord(vaultRunKey(idOrKey))
	if err != nil {
		return err
	}
	summary, err := vaultRunSummary(blob)
	if err != nil {
		return err
	}
	bounded, err := vaultBoundedValue(vaultSummaryTooLargeCode, "the terminal summary of "+vaultRunKey(idOrKey), summary, vaultSummaryMaxBytes)
	if err != nil {
		return err
	}
	if bounded != "" {
		cmd.Println(bounded)
	}
	return nil
}
