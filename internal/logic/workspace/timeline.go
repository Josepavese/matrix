package workspace

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/google/uuid"
)

const (
	EventKeyPrefix       = "workspace.event."
	TimelineKeyPrefix    = "workspace.timeline."
	StateKeyPrefix       = "workspace.state."
	maxTimelineEventRefs = 200
)

// Event records a meaningful operational transition for one workspace.
type Event struct {
	ID               string                 `json:"id"`
	WorkspaceID      string                 `json:"workspace_id"`
	Type             string                 `json:"type"`
	ChannelID        string                 `json:"channel_id,omitempty"`
	LogicalSessionID string                 `json:"logical_session_id,omitempty"`
	RemoteSessionID  string                 `json:"remote_session_id,omitempty"`
	AgentID          string                 `json:"agent_id,omitempty"`
	Mode             string                 `json:"mode,omitempty"`
	Message          string                 `json:"message,omitempty"`
	Reason           string                 `json:"reason,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
}

// State is the materialized current workspace state derived from timeline events
// and mirrored session metadata.
type State struct {
	WorkspaceID            string                 `json:"workspace_id"`
	ActiveLogicalSessionID string                 `json:"active_logical_session_id,omitempty"`
	ActiveRemoteSessionID  string                 `json:"active_remote_session_id,omitempty"`
	ActiveAgentID          string                 `json:"active_agent_id,omitempty"`
	ActiveMode             string                 `json:"active_mode,omitempty"`
	RemoteStatus           string                 `json:"remote_status,omitempty"`
	LastEventType          string                 `json:"last_event_type,omitempty"`
	LastEventMessage       string                 `json:"last_event_message,omitempty"`
	LastEventAt            time.Time              `json:"last_event_at,omitempty"`
	LastHandoff            map[string]interface{} `json:"last_handoff,omitempty"`
	LastDecision           map[string]interface{} `json:"last_decision,omitempty"`
}

func EventKey(workspaceID, eventID string) string {
	return EventKeyPrefix + workspaceID + "." + eventID
}

func TimelineKey(workspaceID string) string {
	return TimelineKeyPrefix + workspaceID
}

func StateKey(workspaceID string) string {
	return StateKeyPrefix + workspaceID
}

// RecordEvent appends a workspace event and updates materialized current state.
func RecordEvent(storage middleware.Storage, event Event) (Event, error) {
	if storage == nil {
		return Event{}, fmt.Errorf("storage not available")
	}
	event.WorkspaceID = strings.TrimSpace(event.WorkspaceID)
	event.Type = strings.TrimSpace(event.Type)
	if event.WorkspaceID == "" {
		return Event{}, fmt.Errorf("workspace id is required")
	}
	if event.Type == "" {
		return Event{}, fmt.Errorf("event type is required")
	}
	if strings.TrimSpace(event.ID) == "" {
		event.ID = uuid.NewString()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return Event{}, fmt.Errorf("failed to encode workspace event: %w", err)
	}
	if err := storage.Set(EventKey(event.WorkspaceID, event.ID), payload); err != nil {
		return Event{}, fmt.Errorf("failed to store workspace event: %w", err)
	}
	evicted, err := updateStringIndexWithLimitEvicted(storage, TimelineKey(event.WorkspaceID), event.ID, maxTimelineEventRefs)
	if err != nil {
		return Event{}, err
	}
	for _, eventID := range evicted {
		if err := storage.Delete(EventKey(event.WorkspaceID, eventID)); err != nil {
			return Event{}, fmt.Errorf("failed to prune evicted workspace event %s: %w", eventID, err)
		}
	}

	state, _, err := LoadState(storage, event.WorkspaceID)
	if err != nil {
		return Event{}, err
	}
	nextState := applyEventToState(state, event)
	if err := SaveState(storage, nextState); err != nil {
		return Event{}, err
	}
	return event, nil
}

// LoadEvent returns one workspace event by id.
func LoadEvent(storage middleware.Storage, workspaceID, eventID string) (Event, bool, error) {
	if storage == nil {
		return Event{}, false, fmt.Errorf("storage not available")
	}
	data, err := storage.Get(EventKey(workspaceID, eventID))
	if err != nil {
		return Event{}, false, fmt.Errorf("failed to read workspace event %s: %w", eventID, err)
	}
	if len(data) == 0 {
		return Event{}, false, nil
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return Event{}, false, fmt.Errorf("failed to decode workspace event %s: %w", eventID, err)
	}
	return event, true, nil
}

// LoadTimeline returns recent workspace events newest-first.
func LoadTimeline(storage middleware.Storage, workspaceID string, limit int) ([]Event, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage not available")
	}
	ids, err := loadStringIndex(storage, TimelineKey(workspaceID))
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	events := make([]Event, 0, len(ids))
	for _, eventID := range ids {
		event, found, err := LoadEvent(storage, workspaceID, eventID)
		if err != nil {
			return nil, err
		}
		if found {
			events = append(events, event)
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].CreatedAt.After(events[j].CreatedAt)
	})
	return events, nil
}

// SaveState persists the materialized current state for a workspace.
func SaveState(storage middleware.Storage, state State) error {
	if storage == nil {
		return fmt.Errorf("storage not available")
	}
	if strings.TrimSpace(state.WorkspaceID) == "" {
		return fmt.Errorf("workspace id is required")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to encode workspace state %s: %w", state.WorkspaceID, err)
	}
	if err := storage.Set(StateKey(state.WorkspaceID), data); err != nil {
		return fmt.Errorf("failed to store workspace state %s: %w", state.WorkspaceID, err)
	}
	return nil
}

// LoadState returns the materialized current state for a workspace.
func LoadState(storage middleware.Storage, workspaceID string) (State, bool, error) {
	if storage == nil {
		return State{}, false, fmt.Errorf("storage not available")
	}
	data, err := storage.Get(StateKey(workspaceID))
	if err != nil {
		return State{}, false, fmt.Errorf("failed to read workspace state %s: %w", workspaceID, err)
	}
	if len(data) == 0 {
		return State{}, false, nil
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, false, fmt.Errorf("failed to decode workspace state %s: %w", workspaceID, err)
	}
	return state, true, nil
}

func applyEventToState(state State, event Event) State {
	if state.WorkspaceID == "" {
		state.WorkspaceID = event.WorkspaceID
	}
	state.LastEventType = event.Type
	state.LastEventMessage = event.Message
	state.LastEventAt = event.CreatedAt

	if event.LogicalSessionID != "" {
		state.ActiveLogicalSessionID = event.LogicalSessionID
	}
	if event.RemoteSessionID != "" {
		state.ActiveRemoteSessionID = event.RemoteSessionID
	}
	if event.AgentID != "" {
		state.ActiveAgentID = event.AgentID
	}
	if event.Mode != "" {
		state.ActiveMode = event.Mode
	}

	switch event.Type {
	case "session.canceled":
		state.RemoteStatus = "canceled"
	case "session.deleted":
		state.RemoteStatus = "deleted"
	case "handoff.created", "handoff.applied":
		state.LastHandoff = extractHandoffMetadata(event)
		if state.RemoteStatus == "" {
			state.RemoteStatus = "active"
		}
	case "decision.recorded":
		state.LastDecision = extractDecisionMetadata(event)
	default:
		if event.Message != "" && state.RemoteStatus == "" {
			state.RemoteStatus = "active"
		}
	}

	if rawStatus, ok := event.Metadata["remote_status"]; ok {
		if status, ok := rawStatus.(string); ok && strings.TrimSpace(status) != "" {
			state.RemoteStatus = status
		}
	}

	return state
}

func extractHandoffMetadata(event Event) map[string]interface{} {
	if len(event.Metadata) == 0 {
		return nil
	}
	handoff := map[string]interface{}{}
	for _, key := range []string{"from_agent_id", "to_agent_id", "summary"} {
		if value, ok := event.Metadata[key]; ok {
			handoff[key] = value
		}
	}
	if len(handoff) == 0 {
		return nil
	}
	return handoff
}

func extractDecisionMetadata(event Event) map[string]interface{} {
	if len(event.Metadata) == 0 {
		return nil
	}
	decision := map[string]interface{}{}
	for _, key := range []string{
		"kind",
		"source",
		"explanation",
		"requested_agent_id",
		"selected_agent_id",
		"selected_session_id",
		"selected_mode",
		"fallback_used",
	} {
		if value, ok := event.Metadata[key]; ok {
			decision[key] = value
		}
	}
	if len(decision) == 0 {
		return nil
	}
	decision["created_at"] = event.CreatedAt.Format(time.RFC3339)
	return decision
}

func updateStringIndexWithLimit(storage middleware.Storage, key, value string, maxLen int) error {
	_, err := updateStringIndexWithLimitEvicted(storage, key, value, maxLen)
	return err
}

func updateStringIndexWithLimitEvicted(storage middleware.Storage, key, value string, maxLen int) ([]string, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage not available")
	}
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	existing, err := loadStringIndex(storage, key)
	if err != nil {
		return nil, err
	}
	filtered := make([]string, 0, len(existing))
	for _, item := range existing {
		if item != value {
			filtered = append(filtered, item)
		}
	}
	next := append([]string{value}, filtered...)
	var evicted []string
	if maxLen > 0 && len(next) > maxLen {
		evicted = append(evicted, next[maxLen:]...)
		next = next[:maxLen]
	}
	data, err := json.Marshal(next)
	if err != nil {
		return nil, fmt.Errorf("failed to encode workspace index %s: %w", key, err)
	}
	if err := storage.Set(key, data); err != nil {
		return nil, err
	}
	return evicted, nil
}
