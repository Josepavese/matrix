package workspace

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/google/uuid"
)

const (
	TurnKeyPrefix          = "workspace.turn."
	TurnIndexKeyPrefix     = "workspace.turns."
	SnapshotKeyPrefix      = "workspace.snapshot."
	SnapshotIndexKeyPrefix = "workspace.snapshots."
	maxTurnRefs            = 500
	maxSnapshotRefs        = 100
)

// Turn captures one locally mirrored work-memory turn for a workspace.
type Turn struct {
	ID               string    `json:"id"`
	WorkspaceID      string    `json:"workspace_id"`
	LogicalSessionID string    `json:"logical_session_id,omitempty"`
	RemoteSessionID  string    `json:"remote_session_id,omitempty"`
	AgentID          string    `json:"agent_id,omitempty"`
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	CreatedAt        time.Time `json:"created_at"`
}

// Snapshot stores a named checkpoint of current work state.
type Snapshot struct {
	ID                     string                 `json:"id"`
	WorkspaceID            string                 `json:"workspace_id"`
	Title                  string                 `json:"title,omitempty"`
	Note                   string                 `json:"note,omitempty"`
	ActiveLogicalSessionID string                 `json:"active_logical_session_id,omitempty"`
	ActiveRemoteSessionID  string                 `json:"active_remote_session_id,omitempty"`
	ActiveAgentID          string                 `json:"active_agent_id,omitempty"`
	ActiveMode             string                 `json:"active_mode,omitempty"`
	RemoteStatus           string                 `json:"remote_status,omitempty"`
	LastEventType          string                 `json:"last_event_type,omitempty"`
	LastEventAt            time.Time              `json:"last_event_at,omitempty"`
	LastHandoff            map[string]interface{} `json:"last_handoff,omitempty"`
	LastDecision           map[string]interface{} `json:"last_decision,omitempty"`
	TurnIDs                []string               `json:"turn_ids,omitempty"`
	EventIDs               []string               `json:"event_ids,omitempty"`
	CreatedAt              time.Time              `json:"created_at"`
}

func TurnKey(workspaceID, turnID string) string {
	return TurnKeyPrefix + workspaceID + "." + turnID
}

func TurnIndexKey(workspaceID string) string {
	return TurnIndexKeyPrefix + workspaceID
}

func SnapshotKey(workspaceID, snapshotID string) string {
	return SnapshotKeyPrefix + workspaceID + "." + snapshotID
}

func SnapshotIndexKey(workspaceID string) string {
	return SnapshotIndexKeyPrefix + workspaceID
}

func RecordTurn(storage middleware.Storage, turn Turn) (Turn, error) {
	if storage == nil {
		return Turn{}, fmt.Errorf("storage not available")
	}
	turn.WorkspaceID = strings.TrimSpace(turn.WorkspaceID)
	turn.Role = strings.TrimSpace(turn.Role)
	if turn.WorkspaceID == "" {
		return Turn{}, fmt.Errorf("workspace id is required")
	}
	if turn.Role == "" {
		return Turn{}, fmt.Errorf("turn role is required")
	}
	if strings.TrimSpace(turn.Content) == "" {
		return Turn{}, fmt.Errorf("turn content is required")
	}
	if turn.ID == "" {
		turn.ID = uuid.NewString()
	}
	if turn.CreatedAt.IsZero() {
		turn.CreatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(turn)
	if err != nil {
		return Turn{}, fmt.Errorf("failed to encode workspace turn: %w", err)
	}
	if err := storage.Set(TurnKey(turn.WorkspaceID, turn.ID), data); err != nil {
		return Turn{}, fmt.Errorf("failed to store workspace turn: %w", err)
	}
	evicted, err := updateStringIndexWithLimitEvicted(storage, TurnIndexKey(turn.WorkspaceID), turn.ID, maxTurnRefs)
	if err != nil {
		return Turn{}, err
	}
	for _, turnID := range evicted {
		if err := storage.Delete(TurnKey(turn.WorkspaceID, turnID)); err != nil {
			return Turn{}, fmt.Errorf("failed to prune evicted workspace turn %s: %w", turnID, err)
		}
	}
	return turn, nil
}

func LoadTurn(storage middleware.Storage, workspaceID, turnID string) (Turn, bool, error) {
	if storage == nil {
		return Turn{}, false, fmt.Errorf("storage not available")
	}
	data, err := storage.Get(TurnKey(workspaceID, turnID))
	if err != nil {
		return Turn{}, false, fmt.Errorf("failed to read workspace turn %s: %w", turnID, err)
	}
	if len(data) == 0 {
		return Turn{}, false, nil
	}
	var turn Turn
	if err := json.Unmarshal(data, &turn); err != nil {
		return Turn{}, false, fmt.Errorf("failed to decode workspace turn %s: %w", turnID, err)
	}
	return turn, true, nil
}

func LoadTurns(storage middleware.Storage, workspaceID string, limit int) ([]Turn, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage not available")
	}
	ids, err := loadStringIndex(storage, TurnIndexKey(workspaceID))
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	turns := make([]Turn, 0, len(ids))
	for _, turnID := range ids {
		turn, found, err := LoadTurn(storage, workspaceID, turnID)
		if err != nil {
			return nil, err
		}
		if found {
			turns = append(turns, turn)
		}
	}
	return turns, nil
}

func SaveSnapshot(storage middleware.Storage, snapshot Snapshot) (Snapshot, error) {
	if storage == nil {
		return Snapshot{}, fmt.Errorf("storage not available")
	}
	snapshot.WorkspaceID = strings.TrimSpace(snapshot.WorkspaceID)
	if snapshot.WorkspaceID == "" {
		return Snapshot{}, fmt.Errorf("workspace id is required")
	}
	if snapshot.ID == "" {
		snapshot.ID = uuid.NewString()
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return Snapshot{}, fmt.Errorf("failed to encode workspace snapshot: %w", err)
	}
	if err := storage.Set(SnapshotKey(snapshot.WorkspaceID, snapshot.ID), data); err != nil {
		return Snapshot{}, fmt.Errorf("failed to store workspace snapshot: %w", err)
	}
	evicted, err := updateStringIndexWithLimitEvicted(storage, SnapshotIndexKey(snapshot.WorkspaceID), snapshot.ID, maxSnapshotRefs)
	if err != nil {
		return Snapshot{}, err
	}
	for _, snapshotID := range evicted {
		if err := storage.Delete(SnapshotKey(snapshot.WorkspaceID, snapshotID)); err != nil {
			return Snapshot{}, fmt.Errorf("failed to prune evicted workspace snapshot %s: %w", snapshotID, err)
		}
	}
	return snapshot, nil
}

func LoadSnapshot(storage middleware.Storage, workspaceID, snapshotID string) (Snapshot, bool, error) {
	if storage == nil {
		return Snapshot{}, false, fmt.Errorf("storage not available")
	}
	data, err := storage.Get(SnapshotKey(workspaceID, snapshotID))
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("failed to read workspace snapshot %s: %w", snapshotID, err)
	}
	if len(data) == 0 {
		return Snapshot{}, false, nil
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, false, fmt.Errorf("failed to decode workspace snapshot %s: %w", snapshotID, err)
	}
	return snapshot, true, nil
}

func LoadSnapshots(storage middleware.Storage, workspaceID string, limit int) ([]Snapshot, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage not available")
	}
	ids, err := loadStringIndex(storage, SnapshotIndexKey(workspaceID))
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	snapshots := make([]Snapshot, 0, len(ids))
	for _, snapshotID := range ids {
		snapshot, found, err := LoadSnapshot(storage, workspaceID, snapshotID)
		if err != nil {
			return nil, err
		}
		if found {
			snapshots = append(snapshots, snapshot)
		}
	}
	return snapshots, nil
}
