package workspace

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

const (
	RetentionTimelineMaxKey  = "retention.workspace.timeline_max"
	RetentionMemoryMaxKey    = "retention.workspace.memory_max"
	RetentionSnapshotsMaxKey = "retention.workspace.snapshots_max"
)

type RetentionPolicy struct {
	TimelineMax  int `json:"timeline_max"`
	MemoryMax    int `json:"memory_max"`
	SnapshotsMax int `json:"snapshots_max"`
}

type PruneReport struct {
	WorkspaceID      string `json:"workspace_id"`
	TimelineBefore   int    `json:"timeline_before"`
	TimelineAfter    int    `json:"timeline_after"`
	TimelineRemoved  int    `json:"timeline_removed"`
	MemoryBefore     int    `json:"memory_before"`
	MemoryAfter      int    `json:"memory_after"`
	MemoryRemoved    int    `json:"memory_removed"`
	SnapshotsBefore  int    `json:"snapshots_before"`
	SnapshotsAfter   int    `json:"snapshots_after"`
	SnapshotsRemoved int    `json:"snapshots_removed"`
}

func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		TimelineMax:  maxTimelineEventRefs,
		MemoryMax:    maxTurnRefs,
		SnapshotsMax: maxSnapshotRefs,
	}
}

func LoadRetentionPolicy(storage middleware.Storage) (RetentionPolicy, error) {
	policy := DefaultRetentionPolicy()
	if storage == nil {
		return policy, fmt.Errorf("storage not available")
	}
	if value, err := loadOptionalInt(storage, RetentionTimelineMaxKey); err != nil {
		return policy, err
	} else if value > 0 {
		policy.TimelineMax = value
	}
	if value, err := loadOptionalInt(storage, RetentionMemoryMaxKey); err != nil {
		return policy, err
	} else if value > 0 {
		policy.MemoryMax = value
	}
	if value, err := loadOptionalInt(storage, RetentionSnapshotsMaxKey); err != nil {
		return policy, err
	} else if value > 0 {
		policy.SnapshotsMax = value
	}
	return policy, nil
}

func SaveRetentionPolicy(storage middleware.Storage, policy RetentionPolicy) error {
	if storage == nil {
		return fmt.Errorf("storage not available")
	}
	if policy.TimelineMax <= 0 || policy.MemoryMax <= 0 || policy.SnapshotsMax <= 0 {
		return fmt.Errorf("retention policy values must be positive")
	}
	if err := saveOptionalInt(storage, RetentionTimelineMaxKey, policy.TimelineMax); err != nil {
		return err
	}
	if err := saveOptionalInt(storage, RetentionMemoryMaxKey, policy.MemoryMax); err != nil {
		return err
	}
	return saveOptionalInt(storage, RetentionSnapshotsMaxKey, policy.SnapshotsMax)
}

func PruneWorkspace(storage middleware.Storage, workspaceID string, policy RetentionPolicy) (PruneReport, error) {
	if storage == nil {
		return PruneReport{}, fmt.Errorf("storage not available")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return PruneReport{}, fmt.Errorf("workspace id is required")
	}

	report := PruneReport{WorkspaceID: workspaceID}

	timelineBefore, timelineAfter, timelineRemoved, err := pruneIndexedObjects(storage, TimelineKey(workspaceID), EventKeyPrefix+workspaceID+".", policy.TimelineMax)
	if err != nil {
		return PruneReport{}, err
	}
	report.TimelineBefore = timelineBefore
	report.TimelineAfter = timelineAfter
	report.TimelineRemoved = timelineRemoved

	memoryBefore, memoryAfter, memoryRemoved, err := pruneIndexedObjects(storage, TurnIndexKey(workspaceID), TurnKeyPrefix+workspaceID+".", policy.MemoryMax)
	if err != nil {
		return PruneReport{}, err
	}
	report.MemoryBefore = memoryBefore
	report.MemoryAfter = memoryAfter
	report.MemoryRemoved = memoryRemoved

	snapshotsBefore, snapshotsAfter, snapshotsRemoved, err := pruneIndexedObjects(storage, SnapshotIndexKey(workspaceID), SnapshotKeyPrefix+workspaceID+".", policy.SnapshotsMax)
	if err != nil {
		return PruneReport{}, err
	}
	report.SnapshotsBefore = snapshotsBefore
	report.SnapshotsAfter = snapshotsAfter
	report.SnapshotsRemoved = snapshotsRemoved

	return report, nil
}

func PruneAllWorkspaces(storage middleware.Storage, policy RetentionPolicy) ([]PruneReport, error) {
	metas, err := ListMeta(storage)
	if err != nil {
		return nil, err
	}
	reports := make([]PruneReport, 0, len(metas))
	for _, meta := range metas {
		report, err := PruneWorkspace(storage, meta.ID, policy)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func WorkspaceRetentionFootprint(storage middleware.Storage, workspaceID string) (PruneReport, error) {
	report := PruneReport{WorkspaceID: workspaceID}
	ids, err := loadStringIndex(storage, TimelineKey(workspaceID))
	if err != nil {
		return report, err
	}
	report.TimelineBefore = len(ids)
	report.TimelineAfter = len(ids)
	ids, err = loadStringIndex(storage, TurnIndexKey(workspaceID))
	if err != nil {
		return report, err
	}
	report.MemoryBefore = len(ids)
	report.MemoryAfter = len(ids)
	ids, err = loadStringIndex(storage, SnapshotIndexKey(workspaceID))
	if err != nil {
		return report, err
	}
	report.SnapshotsBefore = len(ids)
	report.SnapshotsAfter = len(ids)
	return report, nil
}

func pruneIndexedObjects(storage middleware.Storage, indexKey, objectKeyPrefix string, maxLen int) (before, after, removed int, err error) {
	ids, err := loadStringIndex(storage, indexKey)
	if err != nil {
		return 0, 0, 0, err
	}
	before = len(ids)
	if maxLen <= 0 || len(ids) <= maxLen {
		return before, before, 0, nil
	}
	keep := ids[:maxLen]
	remove := ids[maxLen:]
	for _, id := range remove {
		if err := storage.Delete(objectKeyPrefix + id); err != nil {
			return 0, 0, 0, fmt.Errorf("delete %s%s: %w", objectKeyPrefix, id, err)
		}
	}
	data, err := json.Marshal(keep)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("encode index %s: %w", indexKey, err)
	}
	if err := storage.Set(indexKey, data); err != nil {
		return 0, 0, 0, fmt.Errorf("store index %s: %w", indexKey, err)
	}
	return before, len(keep), len(remove), nil
}

func loadOptionalInt(storage middleware.Storage, key string) (int, error) {
	data, err := storage.Get(key)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", key, err)
	}
	if len(data) == 0 {
		return 0, nil
	}
	var value int
	if err := json.Unmarshal(data, &value); err != nil {
		return 0, fmt.Errorf("decode %s: %w", key, err)
	}
	return value, nil
}

func saveOptionalInt(storage middleware.Storage, key string, value int) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", key, err)
	}
	if err := storage.Set(key, data); err != nil {
		return fmt.Errorf("store %s: %w", key, err)
	}
	return nil
}
