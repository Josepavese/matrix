package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentstore"
	"github.com/Josepavese/matrix/internal/logic/matrixhome"
	"github.com/spf13/cobra"
)

var (
	agentStoreRoot  string
	agentStoreJSON  bool
	agentStoreHomes []string
	agentStoreApply bool
)

var agentStoreCmd = &cobra.Command{
	Use:   "store",
	Short: "Manage the shared immutable agent artifact store",
	Long: `Manage the immutable, content-addressed store of agent artifacts shared by many
PAL homes.

A published generation is keyed by the sha256 Matrix already verified at download,
so many homes can run the same verified software without keeping one copy each. The
store lives outside every PAL home; the runtime user must not be able to write it
when it is used for installs.`,
}

var agentStorePublishCmd = &cobra.Command{
	Use:   "publish <agent_id>",
	Short: "Publish the verified binary artifact of an agent into the store",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		root, err := resolveStoreRoot(agentStoreRoot)
		if err != nil {
			exitf("Error: %v", err)
		}
		ctx, cleanup, err := NewInstallerContext(DefaultVaultPath)
		if err != nil {
			exitf("Error: %v", err)
		}
		defer cleanup()

		if agentStoreJSON {
			// Install progress would corrupt the machine-readable report.
			ctx.Installer.SetProgressWriter(nil)
		}
		generation, err := ctx.Installer.PublishToStore(context.Background(), args[0], root)
		if err != nil {
			exitf("Error: %v", err)
		}
		if agentStoreJSON {
			emitJSON(cmd, map[string]any{"store": root, "generation": generation})
			return
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Published %s %s (%s) generation %s\n", generation.AgentID, generation.Version, generation.Platform, generation.Digest)
		fmt.Fprintf(out, "  store:         %s\n", root)
		fmt.Fprintf(out, "  launcher:      %s\n", generation.Launcher)
		fmt.Fprintf(out, "  tree manifest: %s\n", generation.TreeManifestSHA256)
	},
}

var agentStoreListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the published generations of the store",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		store, err := openInspectionStore(agentStoreRoot)
		if err != nil {
			exitf("Error: %v", err)
		}
		generations, err := store.List()
		if err != nil {
			exitf("Error: %v", err)
		}
		if agentStoreJSON {
			emitJSON(cmd, map[string]any{"store": store.Root(), "generations": generations})
			return
		}
		out := cmd.OutOrStdout()
		fmt.Fprintln(out, "DIGEST\tAGENT\tVERSION\tPLATFORM\tPUBLISHED")
		for _, generation := range generations {
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\n", generation.Digest, generation.AgentID,
				generation.Version, generation.Platform, generation.PublishedAt.UTC().Format(time.RFC3339))
		}
	},
}

var agentStoreVerifyCmd = &cobra.Command{
	Use:   "verify <digest>",
	Short: "Re-verify a published generation against its recorded tree manifest",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		store, err := openInspectionStore(agentStoreRoot)
		if err != nil {
			exitf("Error: %v", err)
		}
		digest := strings.TrimSpace(args[0])
		if err := store.Verify(digest); err != nil {
			exitf("Error: store generation %s failed verification: %v", digest, err)
		}
		generation, err := store.Get(digest)
		if err != nil {
			exitf("Error: %v", err)
		}
		if agentStoreJSON {
			emitJSON(cmd, map[string]any{
				"store": store.Root(), "digest": generation.Digest, "verified": true,
				"agent_id": generation.AgentID, "version": generation.Version,
				"platform": generation.Platform, "tree_manifest_sha256": generation.TreeManifestSHA256,
			})
			return
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Verified generation %s (agent %s %s, platform %s)\n", generation.Digest, generation.AgentID, generation.Version, generation.Platform)
		fmt.Fprintf(out, "  tree manifest: %s\n", generation.TreeManifestSHA256)
	},
}

var agentStoreGCCmd = &cobra.Command{
	Use:   "gc --home <path>...",
	Short: "Delete store generations no PAL home references (dry run by default)",
	Long: `Delete generations no PAL home references, plus stale publish staging.

Pass every PAL home whose references must be kept with --home (repeatable): a
generation referenced by any of them is never touched. Dry run by default; --apply
deletes.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		root, err := resolveStoreRoot(agentStoreRoot)
		if err != nil {
			exitf("Error: %v", err)
		}
		home, err := currentPALHome()
		if err != nil {
			exitf("Error: %v", err)
		}
		if len(agentStoreHomes) == 0 {
			exitf("Error: --home is required at least once: garbage collection must know every PAL home whose references to keep")
		}
		refs, err := agentstore.LoadRefSet(agentStoreHomes)
		if err != nil {
			exitf("Error: %v", err)
		}
		pending, freed, err := collectStoreGC(root, home, refs, agentStoreApply)
		if err != nil {
			exitf("Error: %v", err)
		}
		if agentStoreJSON {
			emitJSON(cmd, storeGCReport{Store: root, DryRun: !agentStoreApply, Pending: gcEntries(pending), FreedBytes: freed})
			return
		}
		out := cmd.OutOrStdout()
		fmt.Fprintln(out, "DIGEST\tAGENT\tVERSION\tPLATFORM\tBYTES")
		for _, item := range pending {
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%d\n", item.Digest, item.AgentID, item.Version, item.Platform, item.Bytes)
		}
		if agentStoreApply {
			fmt.Fprintf(out, "Freed %d bytes from %d generation(s)\n", freed, len(pending))
			return
		}
		fmt.Fprintf(out, "Would free %d bytes from %d generation(s) (dry run; pass --apply to delete)\n", freed, len(pending))
	},
}

// storeGCEntry is the JSON form of one reclaimable generation.
type storeGCEntry struct {
	Digest   string `json:"digest"`
	AgentID  string `json:"agent_id"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Bytes    int64  `json:"bytes"`
}

type storeGCReport struct {
	Store      string         `json:"store"`
	DryRun     bool           `json:"dry_run"`
	Pending    []storeGCEntry `json:"pending"`
	FreedBytes int64          `json:"freed_bytes"`
}

func gcEntries(pending []agentstore.Pending) []storeGCEntry {
	entries := make([]storeGCEntry, 0, len(pending))
	for _, item := range pending {
		entries = append(entries, storeGCEntry{
			Digest: item.Digest, AgentID: item.AgentID, Version: item.Version,
			Platform: item.Platform, Bytes: item.Bytes,
		})
	}
	return entries
}

// collectStoreGC reports (dry run) or deletes (apply) unreferenced generations.
// Reporting opens the store read-only when it is sealed; only applying needs a
// publisher.
func collectStoreGC(root, home string, refs agentstore.RefSet, apply bool) ([]agentstore.Pending, int64, error) {
	if !apply {
		store, err := openInspectionStore(root)
		if err != nil {
			return nil, 0, err
		}
		pending, err := store.Reclaimable(refs)
		if err != nil {
			return nil, 0, err
		}
		var bytes int64
		for _, item := range pending {
			bytes += item.Bytes
		}
		return pending, bytes, nil
	}
	store, err := agentstore.OpenPublisher(root, home)
	if err != nil {
		return nil, 0, err
	}
	return store.GC(refs)
}

// resolveStoreRoot resolves the --store flag with the deployment contract as the
// fallback: $MATRIX_AGENT_STORE.
func resolveStoreRoot(flagValue string) (string, error) {
	root := strings.TrimSpace(flagValue)
	if root == "" {
		root = strings.TrimSpace(os.Getenv(agentstore.EnvName))
	}
	if root == "" {
		return "", fmt.Errorf("store path is required: pass --store or set %s", agentstore.EnvName)
	}
	return root, nil
}

// openInspectionStore opens a store for read-only inspection. A sealed store is
// opened as a consumer; a writable publisher layout falls back to the publisher
// role because listing and verifying never mutate anything.
func openInspectionStore(root string) (*agentstore.Store, error) {
	flagRoot, err := resolveStoreRoot(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(flagRoot); err != nil {
		return nil, fmt.Errorf("agent store %s: %w", flagRoot, err)
	}
	home, err := currentPALHome()
	if err != nil {
		return nil, err
	}
	store, consumerErr := agentstore.OpenConsumer(flagRoot, home)
	if consumerErr == nil {
		return store, nil
	}
	publisher, publisherErr := agentstore.OpenPublisher(flagRoot, home)
	if publisherErr == nil {
		return publisher, nil
	}
	return nil, fmt.Errorf("cannot open agent store %s: %w", flagRoot, consumerErr)
}

// currentPALHome is the PAL home a store must live outside of.
func currentPALHome() (string, error) {
	if strings.TrimSpace(activeMatrixHome) != "" {
		return activeMatrixHome, nil
	}
	return matrixhome.Resolve()
}

func init() {
	for _, command := range []*cobra.Command{agentStorePublishCmd, agentStoreListCmd, agentStoreVerifyCmd, agentStoreGCCmd} {
		command.Flags().StringVar(&agentStoreRoot, "store", "", "Agent store root (defaults to $"+agentstore.EnvName+")")
		command.Flags().BoolVar(&agentStoreJSON, "json", false, "Emit the result as JSON")
	}
	agentStoreGCCmd.Flags().StringArrayVar(&agentStoreHomes, "home", nil, "PAL home whose references must be kept (repeatable)")
	agentStoreGCCmd.Flags().BoolVar(&agentStoreApply, "apply", false, "Delete the unreferenced generations (dry run otherwise)")
	agentStoreCmd.AddCommand(agentStorePublishCmd, agentStoreListCmd, agentStoreVerifyCmd, agentStoreGCCmd)
}
