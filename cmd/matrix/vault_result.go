package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var vaultResultCmd = &cobra.Command{
	Use:   "result [run id]",
	Short: "Print the diagnostic terminal outcome of a run",
	Long: `Print the diagnostic terminal outcome of a run: status, stop reason, error and
the reference the terminal summary lives at.

The summary content is deliberately not part of this answer — it has its own
getter, ` + "`matrix vault summary <id>`" + ` — and neither is the provider
transcript, which the run record does not carry.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runVaultResult(cmd, args[0]); err != nil {
			exitVaultReadError(err)
		}
	},
}

func init() {
	vaultCmd.AddCommand(vaultResultCmd)
}

func runVaultResult(cmd *cobra.Command, idOrKey string) error {
	blob, err := readVaultRecord(vaultRunKey(idOrKey))
	if err != nil {
		return err
	}
	outcome, err := vaultRunOutcome(blob)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(outcome, "", "  ")
	if err != nil {
		return fmt.Errorf("ERR_VAULT_SERIALIZE %w", err)
	}
	cmd.Println(strings.TrimRight(string(encoded), "\n"))
	return nil
}
