package session

import (
	"github.com/Josepavese/matrix/internal/middleware"
	"testing"
	"time"
)

func TestSessionMetadataPatchClearsFieldsWithoutRevivingOldValues(t *testing.T) {
	meta := &SessionMeta{RemoteTitle: "old", RemoteUpdatedAt: time.Unix(1, 0), RemoteMeta: map[string]interface{}{"drop": 1, "keep": 2}}
	empty := ""
	patch := &middleware.SessionMetadataPatch{Title: &empty, UpdatedAt: &empty, Metadata: map[string]interface{}{"drop": nil, "new": 3}}
	if !applyAgentSessionMetadata(meta, middleware.ConversationMetadata{SessionPatch: patch}, time.Unix(2, 0)) {
		t.Fatal("session patch not applied")
	}
	if meta.RemoteTitle != "" || !meta.RemoteUpdatedAt.IsZero() || meta.RemoteMeta["drop"] != nil || meta.RemoteMeta["keep"] != 2 || meta.RemoteMeta["new"] != 3 {
		t.Fatalf("session clearing lost: %+v", meta)
	}
	if applySessionMetadataPatch(meta, patch) {
		t.Fatal("repeated patch must be idempotent")
	}
	if !applySessionMetadataPatch(meta, &middleware.SessionMetadataPatch{ClearMetadata: true, Metadata: map[string]interface{}{"fresh": true}}) || len(meta.RemoteMeta) != 1 || meta.RemoteMeta["fresh"] != true {
		t.Fatal("whole metadata clear/replace failed")
	}
}
func TestSessionMetadataPatchRejectsInvalidTimestamp(t *testing.T) {
	meta := &SessionMeta{RemoteUpdatedAt: time.Unix(1, 0)}
	bad := "invalid"
	if applySessionMetadataPatch(meta, &middleware.SessionMetadataPatch{UpdatedAt: &bad}) || !meta.RemoteUpdatedAt.Equal(time.Unix(1, 0)) {
		t.Fatal("invalid optional timestamp changed mirror")
	}
}
