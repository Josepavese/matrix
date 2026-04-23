package main

import (
	"context"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/cmdutil"
	"github.com/Josepavese/matrix/internal/logic/session"
	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/spf13/cobra"
)

var (
	workspaceCmd = &cobra.Command{
		Use:   "workspace",
		Short: "Manage workspace affinity metadata",
	}

	workspaceAddPath            string
	workspaceAddKind            string
	workspaceAddDefaultAgent    string
	workspaceAddReviewerAgent   string
	workspaceAddDefaultMode     string
	workspaceAddPolicy          string
	workspaceSwitchChannelID    string
	workspaceTimelineLimit      int
	workspaceDecisionLimit      int
	workspaceMemoryLimit        int
	workspaceSnapshotsLimit     int
	workspaceRetentionAll       bool
	workspaceRetentionSet       bool
	workspaceRetentionTimeline  int
	workspaceRetentionMemory    int
	workspaceRetentionSnapshots int

	workspaceAddCmd = &cobra.Command{
		Use:   "add <workspace-id>",
		Short: "Create or update a workspace",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, closeFn, err := NewAppContext(DefaultVaultPath)
			if err != nil {
				exitf("failed to open vault: %v", err)
			}
			defer closeFn()

			workspaceID := strings.TrimSpace(args[0])
			if workspaceID == "" {
				exitf("workspace id is required")
			}

			meta := workspace.Meta{
				ID:              workspaceID,
				Name:            workspaceID,
				Kind:            workspaceAddKind,
				RootPath:        strings.TrimSpace(workspaceAddPath),
				DefaultAgentID:  strings.TrimSpace(workspaceAddDefaultAgent),
				ReviewerAgentID: strings.TrimSpace(workspaceAddReviewerAgent),
				DefaultMode:     strings.TrimSpace(workspaceAddDefaultMode),
				PolicyProfile:   strings.TrimSpace(workspaceAddPolicy),
			}
			if meta.Kind == "" {
				meta.Kind = "repository"
			}
			if err := workspace.SaveMeta(ctx.Store, meta); err != nil {
				exitf("failed to save workspace: %v", err)
			}
			cmd.Printf("workspace saved: %s\n", workspaceID)
		},
	}

	workspaceListCmd = &cobra.Command{
		Use:   "list",
		Short: "List configured workspaces",
		Run: func(cmd *cobra.Command, _ []string) {
			ctx, closeFn, err := NewReadOnlyAppContext(DefaultVaultPath)
			if err != nil {
				exitf("failed to open vault: %v", err)
			}
			defer closeFn()

			metas, err := workspace.ListMeta(ctx.Store)
			if err != nil {
				exitf("failed to list workspaces: %v", err)
			}
			if err := cmdutil.PrintJSON(cmd, metas); err != nil {
				exitf("failed to print workspaces: %v", err)
			}
		},
	}

	workspaceShowCmd = &cobra.Command{
		Use:   "show <workspace-id>",
		Short: "Show one workspace and its recent session index",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, closeFn, err := NewReadOnlyAppContext(DefaultVaultPath)
			if err != nil {
				exitf("failed to open vault: %v", err)
			}
			defer closeFn()

			meta, found, err := workspace.LoadMeta(ctx.Store, args[0])
			if err != nil {
				exitf("failed to read workspace: %v", err)
			}
			if !found {
				exitf("workspace not found: %s", args[0])
			}
			sessions, err := workspace.LoadSessionIndex(ctx.Store, args[0])
			if err != nil {
				exitf("failed to read workspace session index: %v", err)
			}
			payload := map[string]any{
				"workspace":       meta,
				"recent_sessions": sessions,
			}
			if err := cmdutil.PrintJSON(cmd, payload); err != nil {
				exitf("failed to print workspace: %v", err)
			}
		},
	}

	workspaceSwitchCmd = &cobra.Command{
		Use:   "switch <workspace-id>",
		Short: "Switch a channel onto a workspace-aware session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if strings.TrimSpace(workspaceSwitchChannelID) == "" {
				exitf("--channel is required")
			}
			ctx, closeFn, err := NewAppContext(DefaultVaultPath)
			if err != nil {
				exitf("failed to open vault: %v", err)
			}
			defer closeFn()

			mgr := session.NewManager(ctx.Store, nil, nil, nil)
			meta, found, err := mgr.SwitchWorkspaceForChannel(context.Background(), workspaceSwitchChannelID, args[0])
			if err != nil {
				exitf("failed to switch workspace: %v", err)
			}
			if !found {
				exitf("workspace not found: %s", args[0])
			}
			payload := map[string]any{
				"channel_id": workspaceSwitchChannelID,
				"workspace":  args[0],
				"session":    meta,
			}
			if err := cmdutil.PrintJSON(cmd, payload); err != nil {
				exitf("failed to print workspace switch result: %v", err)
			}
		},
	}

	workspaceStateCmd = &cobra.Command{
		Use:   "state <workspace-id>",
		Short: "Show the current materialized state of a workspace",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, closeFn, err := NewReadOnlyAppContext(DefaultVaultPath)
			if err != nil {
				exitf("failed to open vault: %v", err)
			}
			defer closeFn()

			state, found, err := workspace.LoadState(ctx.Store, args[0])
			if err != nil {
				exitf("failed to read workspace state: %v", err)
			}
			if !found {
				exitf("workspace state not found: %s", args[0])
			}
			if err := cmdutil.PrintJSON(cmd, state); err != nil {
				exitf("failed to print workspace state: %v", err)
			}
		},
	}

	workspaceTimelineCmd = &cobra.Command{
		Use:   "timeline <workspace-id>",
		Short: "Show recent timeline events for a workspace",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, closeFn, err := NewReadOnlyAppContext(DefaultVaultPath)
			if err != nil {
				exitf("failed to open vault: %v", err)
			}
			defer closeFn()

			events, err := workspace.LoadTimeline(ctx.Store, args[0], workspaceTimelineLimit)
			if err != nil {
				exitf("failed to read workspace timeline: %v", err)
			}
			if err := cmdutil.PrintJSON(cmd, events); err != nil {
				exitf("failed to print workspace timeline: %v", err)
			}
		},
	}

	workspaceMemoryCmd = &cobra.Command{
		Use:   "memory <workspace-id>",
		Short: "Show recent stored work memory turns for a workspace",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, closeFn, err := NewReadOnlyAppContext(DefaultVaultPath)
			if err != nil {
				exitf("failed to open vault: %v", err)
			}
			defer closeFn()

			turns, err := workspace.LoadTurns(ctx.Store, args[0], workspaceMemoryLimit)
			if err != nil {
				exitf("failed to read workspace memory: %v", err)
			}
			if err := cmdutil.PrintJSON(cmd, turns); err != nil {
				exitf("failed to print workspace memory: %v", err)
			}
		},
	}

	workspaceSnapshotsCmd = &cobra.Command{
		Use:   "snapshots <workspace-id>",
		Short: "Show recent snapshots for a workspace",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, closeFn, err := NewReadOnlyAppContext(DefaultVaultPath)
			if err != nil {
				exitf("failed to open vault: %v", err)
			}
			defer closeFn()

			snapshots, err := workspace.LoadSnapshots(ctx.Store, args[0], workspaceSnapshotsLimit)
			if err != nil {
				exitf("failed to read workspace snapshots: %v", err)
			}
			if err := cmdutil.PrintJSON(cmd, snapshots); err != nil {
				exitf("failed to print workspace snapshots: %v", err)
			}
		},
	}

	workspaceDecisionsCmd = &cobra.Command{
		Use:   "decisions <workspace-id>",
		Short: "Show recent orchestration decisions for a workspace",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, closeFn, err := NewReadOnlyAppContext(DefaultVaultPath)
			if err != nil {
				exitf("failed to open vault: %v", err)
			}
			defer closeFn()

			events, err := workspace.LoadTimeline(ctx.Store, args[0], 100)
			if err != nil {
				exitf("failed to read workspace timeline: %v", err)
			}
			decisions := make([]map[string]any, 0, workspaceDecisionLimit)
			for _, event := range events {
				if event.Type != "decision.recorded" {
					continue
				}
				item := map[string]any{
					"kind":                event.Metadata["kind"],
					"source":              event.Metadata["source"],
					"explanation":         event.Metadata["explanation"],
					"requested_agent_id":  event.Metadata["requested_agent_id"],
					"selected_agent_id":   event.Metadata["selected_agent_id"],
					"selected_session_id": event.Metadata["selected_session_id"],
					"selected_mode":       event.Metadata["selected_mode"],
					"fallback_used":       event.Metadata["fallback_used"],
					"created_at":          event.CreatedAt,
				}
				decisions = append(decisions, item)
				if workspaceDecisionLimit > 0 && len(decisions) >= workspaceDecisionLimit {
					break
				}
			}
			if err := cmdutil.PrintJSON(cmd, decisions); err != nil {
				exitf("failed to print workspace decisions: %v", err)
			}
		},
	}

	workspacePruneCmd = &cobra.Command{
		Use:   "prune [workspace-id]",
		Short: "Prune workspace timeline, memory, and snapshots using the configured retention policy",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, closeFn, err := NewAppContext(DefaultVaultPath)
			if err != nil {
				exitf("failed to open vault: %v", err)
			}
			defer closeFn()

			policy, err := workspace.LoadRetentionPolicy(ctx.Store)
			if err != nil {
				exitf("failed to load retention policy: %v", err)
			}
			if workspaceRetentionTimeline > 0 {
				policy.TimelineMax = workspaceRetentionTimeline
			}
			if workspaceRetentionMemory > 0 {
				policy.MemoryMax = workspaceRetentionMemory
			}
			if workspaceRetentionSnapshots > 0 {
				policy.SnapshotsMax = workspaceRetentionSnapshots
			}
			var reports any
			switch {
			case workspaceRetentionAll:
				pruned, err := workspace.PruneAllWorkspaces(ctx.Store, policy)
				if err != nil {
					exitf("failed to prune workspaces: %v", err)
				}
				reports = pruned
			case len(args) == 1:
				pruned, err := workspace.PruneWorkspace(ctx.Store, args[0], policy)
				if err != nil {
					exitf("failed to prune workspace: %v", err)
				}
				reports = pruned
			default:
				exitf("provide a workspace id or use --all")
			}
			if err := cmdutil.PrintJSON(cmd, map[string]any{"policy": policy, "report": reports}); err != nil {
				exitf("failed to print prune report: %v", err)
			}
		},
	}

	workspaceRetentionCmd = &cobra.Command{
		Use:   "retention",
		Short: "Show or update workspace retention policy",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			ctx, closeFn, err := NewAppContext(DefaultVaultPath)
			if err != nil {
				exitf("failed to open vault: %v", err)
			}
			defer closeFn()

			policy, err := workspace.LoadRetentionPolicy(ctx.Store)
			if err != nil {
				exitf("failed to load retention policy: %v", err)
			}
			if workspaceRetentionSet {
				if workspaceRetentionTimeline > 0 {
					policy.TimelineMax = workspaceRetentionTimeline
				}
				if workspaceRetentionMemory > 0 {
					policy.MemoryMax = workspaceRetentionMemory
				}
				if workspaceRetentionSnapshots > 0 {
					policy.SnapshotsMax = workspaceRetentionSnapshots
				}
				if err := workspace.SaveRetentionPolicy(ctx.Store, policy); err != nil {
					exitf("failed to save retention policy: %v", err)
				}
			}
			if err := cmdutil.PrintJSON(cmd, policy); err != nil {
				exitf("failed to print retention policy: %v", err)
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(workspaceCmd)
	workspaceCmd.AddCommand(workspaceAddCmd)
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceShowCmd)
	workspaceCmd.AddCommand(workspaceSwitchCmd)
	workspaceCmd.AddCommand(workspaceStateCmd)
	workspaceCmd.AddCommand(workspaceTimelineCmd)
	workspaceCmd.AddCommand(workspacePruneCmd)
	workspaceCmd.AddCommand(workspaceRetentionCmd)
	workspaceCmd.AddCommand(workspaceDecisionsCmd)
	workspaceCmd.AddCommand(workspaceMemoryCmd)
	workspaceCmd.AddCommand(workspaceSnapshotsCmd)

	workspaceAddCmd.Flags().StringVar(&workspaceAddPath, "path", "", "root path associated with the workspace")
	workspaceAddCmd.Flags().StringVar(&workspaceAddKind, "kind", "repository", "workspace kind")
	workspaceAddCmd.Flags().StringVar(&workspaceAddDefaultAgent, "default-agent", "", "default agent for new sessions in this workspace")
	workspaceAddCmd.Flags().StringVar(&workspaceAddReviewerAgent, "reviewer-agent", "", "reviewer agent for review-mode intents in this workspace")
	workspaceAddCmd.Flags().StringVar(&workspaceAddDefaultMode, "default-mode", "implementation", "default mode for sessions in this workspace")
	workspaceAddCmd.Flags().StringVar(&workspaceAddPolicy, "policy", "", "policy profile for this workspace")
	workspaceSwitchCmd.Flags().StringVar(&workspaceSwitchChannelID, "channel", "", "channel id to switch onto the workspace")
	workspaceTimelineCmd.Flags().IntVar(&workspaceTimelineLimit, "limit", 10, "maximum number of timeline events to show")
	workspaceDecisionsCmd.Flags().IntVar(&workspaceDecisionLimit, "limit", 10, "maximum number of orchestration decisions to show")
	workspaceMemoryCmd.Flags().IntVar(&workspaceMemoryLimit, "limit", 12, "maximum number of memory turns to show")
	workspaceSnapshotsCmd.Flags().IntVar(&workspaceSnapshotsLimit, "limit", 10, "maximum number of snapshots to show")
	workspacePruneCmd.Flags().BoolVar(&workspaceRetentionAll, "all", false, "prune every configured workspace")
	workspacePruneCmd.Flags().IntVar(&workspaceRetentionTimeline, "timeline-max", 0, "override timeline retention during prune")
	workspacePruneCmd.Flags().IntVar(&workspaceRetentionMemory, "memory-max", 0, "override memory retention during prune")
	workspacePruneCmd.Flags().IntVar(&workspaceRetentionSnapshots, "snapshots-max", 0, "override snapshot retention during prune")
	workspaceRetentionCmd.Flags().BoolVar(&workspaceRetentionSet, "set", false, "persist the provided retention values")
	workspaceRetentionCmd.Flags().IntVar(&workspaceRetentionTimeline, "timeline-max", 0, "workspace timeline retention limit")
	workspaceRetentionCmd.Flags().IntVar(&workspaceRetentionMemory, "memory-max", 0, "workspace memory retention limit")
	workspaceRetentionCmd.Flags().IntVar(&workspaceRetentionSnapshots, "snapshots-max", 0, "workspace snapshot retention limit")
}
