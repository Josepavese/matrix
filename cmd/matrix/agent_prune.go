package main

import (
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/spf13/cobra"
)

var (
	agentPruneApply bool
	agentPruneJSON  bool
)

var agentPruneCmd = &cobra.Command{
	Use:   "prune [agent_id...]",
	Short: "Reclaim per-home software copies whose verified store generation still holds",
	Long: `Reclaim the per-home software directory of agents whose software is already a
verified generation of a shared artifact store.

Dry run by default: it prints the bytes that would be reclaimed. --apply removes only
the software directory (configuration and the Vault are untouched). An agent is
refused, and its copy kept, when it has no store reference or the recorded store
generation no longer verifies.`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cleanup, err := NewInstallerContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		ids := args
		if len(ids) == 0 {
			ids, err = agentcfg.ListAgentIDs(ctx.Store)
			if err != nil {
				exitf("Error: %v", err)
			}
		}
		results, failures := pruneAgents(ctx.Installer, ids, agentPruneApply)
		if agentPruneJSON {
			emitJSON(cmd, pruneReport{Results: results, Errors: failures})
		} else {
			for _, line := range pruneLines(ids, results, failures) {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
		}
		if len(failures) > 0 {
			exitf("Error: %d agent(s) refused pruning", len(failures))
		}
	},
}

// pruneReport is the JSON shape of a prune pass: what was reported per agent and
// why an agent was refused. A refusal keeps the copy in place, so it is data the
// caller must see rather than a line on stderr.
type pruneReport struct {
	Results []agentmgr.PruneResult `json:"results"`
	Errors  map[string]string      `json:"errors,omitempty"`
}

func pruneAgents(installer *agentmgr.Installer, ids []string, apply bool) ([]agentmgr.PruneResult, map[string]string) {
	results := make([]agentmgr.PruneResult, 0, len(ids))
	failures := map[string]string{}
	for _, id := range ids {
		result, err := installer.PruneCopy(id, apply)
		if err != nil {
			failures[id] = err.Error()
			continue
		}
		results = append(results, result)
	}
	return results, failures
}

// pruneLines renders the outcomes in the order the agents were named, with a
// refusal line for the agents whose copy was kept.
func pruneLines(ids []string, results []agentmgr.PruneResult, failures map[string]string) []string {
	byID := make(map[string]agentmgr.PruneResult, len(results))
	for _, result := range results {
		byID[result.ID] = result
	}
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		if failure, refused := failures[id]; refused {
			lines = append(lines, fmt.Sprintf("%s\trefused\t%s", id, failure))
			continue
		}
		result, ok := byID[id]
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s\t%d\tremoved=%t\t%s", result.ID, result.Bytes, result.Removed, result.Detail))
	}
	return lines
}

func init() {
	agentPruneCmd.Flags().BoolVar(&agentPruneApply, "apply", false, "Remove the per-home software copies (dry run otherwise)")
	agentPruneCmd.Flags().BoolVar(&agentPruneJSON, "json", false, "Emit the prune results as JSON")
}
