package main

import (
	"github.com/jose/matrix-v2/internal/logic/schema"
	"github.com/jose/matrix-v2/internal/logic/workspace"
	"github.com/jose/matrix-v2/internal/providers/bolt"
)

func buildStorageDoctorReport() (map[string]any, error) {
	provider, err := bolt.NewReadOnlyProvider(DefaultVaultPath)
	if err != nil {
		return map[string]any{
			"schema": map[string]any{
				"status": "unavailable",
				"error":  err.Error(),
			},
		}, nil
	}
	defer func() { _ = provider.Close() }()

	schemaReport, err := schema.LoadReport(provider)
	if err != nil {
		return nil, err
	}
	retentionPolicy, err := workspace.LoadRetentionPolicy(provider)
	if err != nil {
		return nil, err
	}
	metas, err := workspace.ListMeta(provider)
	if err != nil {
		return nil, err
	}
	workspaces := make([]map[string]any, 0, len(metas))
	for _, meta := range metas {
		footprint, err := workspace.WorkspaceRetentionFootprint(provider, meta.ID)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, map[string]any{
			"id":                 meta.ID,
			"timeline_events":    footprint.TimelineBefore,
			"memory_turns":       footprint.MemoryBefore,
			"snapshots":          footprint.SnapshotsBefore,
			"timeline_prunable":  footprint.TimelineBefore > retentionPolicy.TimelineMax,
			"memory_prunable":    footprint.MemoryBefore > retentionPolicy.MemoryMax,
			"snapshots_prunable": footprint.SnapshotsBefore > retentionPolicy.SnapshotsMax,
		})
	}
	return map[string]any{
		"schema":     schemaReport,
		"retention":  retentionPolicy,
		"workspaces": workspaces,
	}, nil
}
