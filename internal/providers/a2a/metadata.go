package a2a

import "encoding/json"

// a2aSafeMetadata projects Matrix metadata onto the data model A2A defines for it.
//
// Why this exists: A2A Metadata is a JSON object - the specification says values "can
// be any valid JSON value" (§3.2.5) and the protocol models the field as
// google.protobuf.Struct. Matrix's routing layer hands a thought notifier the rich,
// protocol-typed metadata it read from the agent: the ACP observer puts the agent's own
// payload (a zedacp.Content struct, []zedacp.Content slices, json.RawMessage) into
// ThoughtUpdate.Metadata, see internal/providers/agents/router_observer_content.go.
// Those Go types are not JSON values, and the protocol SDK's task store rejects them:
// "… is not permitted in Metadata, must be one of nil, bool, int, float, string, []any,
// map[string]any". The rejection happens inside event processing, which fails the task
// and then cancels the context the agent turn is running on - the turn dies with
// "context canceled" and the caller gets a failed task with no message and no artifacts.
//
// The projection is a JSON round trip per value, which is exactly the data model the
// field is defined over. Two consequences are worth knowing: a Go integer arrives at the
// protocol as the double the data model defines, and a protocol-typed value arrives as
// the JSON object or array it serializes to. A value with no JSON representation at all
// - a channel, a function, a NaN - is dropped rather than failing the turn: progress
// metadata is a projection, and losing one key of it must never cost a caller their task.
func a2aSafeMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if projected, ok := a2aJSONValue(value); ok {
			out[key] = projected
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// a2aJSONValue returns value as the JSON data model represents it, or false when the
// value cannot be represented as JSON at all.
func a2aJSONValue(value any) (any, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, false
	}
	return decoded, true
}
