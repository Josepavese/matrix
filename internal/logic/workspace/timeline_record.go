package workspace

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/google/uuid"
)

func normalizeEvent(event Event) (Event, error) {
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
	return event, nil
}

func saveTimelineEvent(storage middleware.Storage, event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to encode workspace event: %w", err)
	}
	if err := storage.Set(EventKey(event.WorkspaceID, event.ID), payload); err != nil {
		return fmt.Errorf("failed to store workspace event: %w", err)
	}
	evicted, err := updateStringIndexWithLimitEvicted(storage, TimelineKey(event.WorkspaceID), event.ID, maxTimelineEventRefs)
	if err != nil {
		return err
	}
	return pruneEvictedObjects(storage, evicted, func(id string) string {
		return EventKey(event.WorkspaceID, id)
	}, "workspace event")
}

func updateTimelineState(storage middleware.Storage, event Event) error {
	state, _, err := LoadState(storage, event.WorkspaceID)
	if err != nil {
		return err
	}
	return SaveState(storage, applyEventToState(state, event))
}
