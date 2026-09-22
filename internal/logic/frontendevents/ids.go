package frontendevents

import (
	"crypto/sha256"
	"encoding/hex"
)

func StableToolCallID(runID, name, content string, metadata map[string]interface{}) string {
	raw := FirstNonEmpty(
		StringValue(metadata, "tool_call_id"),
		StringValue(metadata, "id"),
		StringValue(metadata, "call_id"),
	)
	if raw == "" {
		// The path takes part in the identity: two calls to the same tool with
		// the same description but different arguments are different calls, and
		// collapsing them would mis-link a result to the wrong request. Only
		// identity-bearing fields are hashed, never the whole metadata map, so a
		// replayed update keeps hashing to the same value.
		raw = runID + "|" + name + "|" + content + "|" + StringValue(metadata, "title") + "|" + StringValue(metadata, "path")
	}
	return "tool-" + shortHash(raw)
}

func StablePermissionID(runID, content string) string {
	return "perm-" + shortHash(runID+"|"+content)
}

// shortHash derives a stable correlation identifier. It is an identity, not a
// security primitive: nothing is authenticated or hidden by it, and collision
// resistance only has to be good enough that two calls in one run do not share a
// label. SHA-256 is used anyway so that a reader does not have to reason about
// whether a weak hash matters here.
func shortHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:16]
}
