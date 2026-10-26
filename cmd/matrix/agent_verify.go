package main

import (
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/spf13/cobra"
)

var (
	agentVerifyJSON   bool
	agentVerifyStrict bool
)

var agentVerifyCmd = &cobra.Command{
	Use:   "verify [agent_id...]",
	Short: "Verify installed agent software against its store tree or recorded install evidence",
	Long: `Verify the installed software of the named agents (all agents when none are named).

A store-backed agent is verified by re-reading its published generation and proving
the file tree matches the recorded manifest ("store_tree"). A per-home install is
verified from the evidence recorded at install time and its launcher ("recorded_evidence");
that method never claims the bytes were re-proven offline.`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cleanup, err := NewInstallerContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		results, err := ctx.Installer.VerifyInstalled(args)
		if err != nil {
			exitf("Error: verify: %v", err)
		}
		if agentVerifyJSON {
			emitJSON(cmd, map[string]any{"results": results})
		} else {
			for _, line := range verifyLines(results) {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
		}
		if agentVerifyStrict && verifyAnyFailed(results) {
			exitf("Error: --strict: at least one installed agent did not verify")
		}
	},
}

// verifyLines renders one line per result, naming the method so a reader never
// mistakes recorded evidence for a re-proven tree.
func verifyLines(results []agentmgr.VerifyResult) []string {
	lines := make([]string, 0, len(results))
	for _, result := range results {
		verdict := "OK"
		if !result.Verified {
			verdict = "FAILED"
		}
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s", result.ID, verdict, result.Method, result.Detail))
	}
	return lines
}

func verifyAnyFailed(results []agentmgr.VerifyResult) bool {
	for _, result := range results {
		if !result.Verified {
			return true
		}
	}
	return false
}

func init() {
	agentVerifyCmd.Flags().BoolVar(&agentVerifyJSON, "json", false, "Emit the verification results as JSON")
	agentVerifyCmd.Flags().BoolVar(&agentVerifyStrict, "strict", false, "Exit 1 when any installed agent does not verify")
}
