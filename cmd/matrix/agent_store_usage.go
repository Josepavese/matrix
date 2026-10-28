package main

import (
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/agentstore"
	"github.com/spf13/cobra"
)

var agentStoreUsageHomes []string

var agentStoreUsageCmd = &cobra.Command{
	Use:   "usage --home <path>...",
	Short: "Measure what the store holds, who still references it and what is reclaimable",
	Long: `Measure the published generations of the store together with the per-home
references that still need them.

Every generation is measured on disk and counted once, then classified by how many
of the named PAL homes reference it: exclusive (exactly one), shared (two or more)
or reclaimable (none, so the next gc pass would delete it). Publish staging is
reported apart because it is work in flight, not published software.

This is a measurement of this store and these references. It does not estimate what
a backup would have cost and it does not read the backup consumer's own peak: pass
every home whose references must be counted, exactly as with gc.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		store, err := openInspectionStore(agentStoreRoot)
		if err != nil {
			exitf("Error: %v", err)
		}
		if len(agentStoreUsageHomes) == 0 {
			exitf("Error: --home is required at least once: the measurement must know every PAL home whose references to count")
		}
		fanout, err := agentstore.LoadRefFanout(agentStoreUsageHomes)
		if err != nil {
			exitf("Error: %v", err)
		}
		usage, err := store.Measure(fanout)
		if err != nil {
			exitf("Error: %v", err)
		}
		if agentStoreJSON {
			emitJSON(cmd, usage)
			return
		}
		for _, line := range usageLines(usage) {
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
	},
}

// usageLines renders the measurement as a summary block followed by one line per
// generation. The summary is the part a consumer quotes; the per-generation
// lines are the evidence behind it.
func usageLines(usage agentstore.Usage) []string {
	lines := []string{
		fmt.Sprintf("store: %s", usage.Store),
		fmt.Sprintf("generations: %d", len(usage.Generations)),
		fmt.Sprintf("total: %d bytes", usage.TotalBytes),
		fmt.Sprintf("exclusive: %d bytes (exactly one named home)", usage.ExclusiveBytes),
		fmt.Sprintf("shared: %d bytes (two or more named homes)", usage.SharedBytes),
		fmt.Sprintf("reclaimable: %d bytes (no named home; the next gc pass would delete them)", usage.ReclaimableBytes),
		fmt.Sprintf("staging: %d bytes (publish staging, not published software)", usage.StagingBytes),
		"DIGEST\tAGENT\tVERSION\tPLATFORM\tBYTES\tREFERENCED_BY",
	}
	for _, generation := range usage.Generations {
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s\t%d\t%d",
			generation.Digest, generation.AgentID, generation.Version,
			generation.Platform, generation.Bytes, generation.ReferencedBy))
	}
	return lines
}

func init() {
	agentStoreUsageCmd.Flags().StringVar(&agentStoreRoot, "store", "", "Agent store root (defaults to $"+agentstore.EnvName+")")
	agentStoreUsageCmd.Flags().BoolVar(&agentStoreJSON, "json", false, "Emit the result as JSON")
	agentStoreUsageCmd.Flags().StringArrayVar(&agentStoreUsageHomes, "home", nil, "PAL home whose references to count (repeatable)")
	agentStoreCmd.AddCommand(agentStoreUsageCmd)
}
