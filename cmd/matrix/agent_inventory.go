package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/spf13/cobra"
)

var agentInventoryJSON bool

var agentInventoryCmd = &cobra.Command{
	Use:   "inventory",
	Short: "Print the deterministic, secret-free inventory of installed agent software",
	Long: `Print what software this PAL home has installed and whether it can be regenerated.

The inventory carries no timestamps and no environment values: it names environment
keys and installation evidence, never their contents. Regenerable agents can be
excluded from a data backup and rebuilt with "matrix agent restore".`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		ctx, cleanup, err := NewInstallerContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		inventory, err := ctx.Installer.Inventory()
		if err != nil {
			exitf("Error: inventory: %v", err)
		}
		if agentInventoryJSON {
			emitJSON(cmd, inventory)
			return
		}
		for _, line := range inventoryLines(inventory) {
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
	},
}

// inventoryLines renders the inventory as one header line and one tab-separated
// line per agent, in the deterministic order Inventory already guarantees.
func inventoryLines(inventory agentmgr.Inventory) []string {
	lines := make([]string, 0, len(inventory.Agents)+1)
	lines = append(lines, "ID\tVERSION\tDISTRIBUTION\tARTIFACT\tREGENERABLE\tSTORE")
	for _, agent := range inventory.Agents {
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s",
			agent.ID, dashIfEmpty(agent.Version), dashIfEmpty(agent.Distribution),
			artifactStatus(agent), yesNo(agent.Regenerable), storeDigest(agent)))
	}
	return lines
}

func artifactStatus(agent agentmgr.InventoryAgent) string {
	if agent.Artifact == nil {
		return "not recorded"
	}
	return string(agent.Artifact.Status)
}

func storeDigest(agent agentmgr.InventoryAgent) string {
	if agent.Store == nil {
		return "-"
	}
	return agent.Store.Digest
}

func dashIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

// emitJSON writes one JSON report to the command's stdout, so `--json` output
// stays pipeable.
func emitJSON(cmd *cobra.Command, payload any) {
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		exitf("Error: %v", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(out))
}

func init() {
	agentInventoryCmd.Flags().BoolVar(&agentInventoryJSON, "json", false, "Emit the inventory object as JSON")
}
