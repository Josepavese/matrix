package frontendevents

import (
	"crypto/sha1"
	"encoding/hex"
)

func StableToolCallID(runID, name, content string, metadata map[string]interface{}) string {
	raw := FirstNonEmpty(
		StringValue(metadata, "tool_call_id"),
		StringValue(metadata, "id"),
		StringValue(metadata, "call_id"),
	)
	if raw == "" {
		raw = runID + "|" + name + "|" + content + "|" + StringValue(metadata, "title")
	}
	return "tool-" + shortHash(raw)
}

func StablePermissionID(runID, content string) string {
	return "perm-" + shortHash(runID+"|"+content)
}

func shortHash(raw string) string {
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])[:16]
}
