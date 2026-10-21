package session

import (
	"github.com/Josepavese/matrix/internal/middleware"
	"reflect"
	"time"
)

func applySessionMetadataPatch(meta *SessionMeta, patch *middleware.SessionMetadataPatch) bool {
	if patch == nil {
		return false
	}
	changed := false
	if patch.Title != nil && meta.RemoteTitle != *patch.Title {
		meta.RemoteTitle = *patch.Title
		changed = true
	}
	changed = applySessionTimestampPatch(meta, patch.UpdatedAt) || changed
	if patch.ClearMetadata && len(meta.RemoteMeta) > 0 {
		meta.RemoteMeta = nil
		changed = true
	}
	return applySessionMetadataKeys(meta, patch.Metadata) || changed
}
func applySessionTimestampPatch(meta *SessionMeta, value *string) bool {
	if value == nil {
		return false
	}
	stamp := time.Time{}
	if *value != "" {
		parsed, ok := parseRemoteUpdatedAt(*value)
		if !ok {
			return false
		}
		stamp = parsed
	}
	if stamp.Equal(meta.RemoteUpdatedAt) {
		return false
	}
	meta.RemoteUpdatedAt = stamp
	return true
}
func applySessionMetadataKeys(meta *SessionMeta, patch map[string]interface{}) bool {
	changed := false
	for key, value := range patch {
		old, exists := meta.RemoteMeta[key]
		if value == nil {
			if exists {
				delete(meta.RemoteMeta, key)
				changed = true
			}
			continue
		}
		if exists && reflect.DeepEqual(old, value) {
			continue
		}
		if meta.RemoteMeta == nil {
			meta.RemoteMeta = map[string]interface{}{}
		}
		meta.RemoteMeta[key] = value
		changed = true
	}
	return changed
}
