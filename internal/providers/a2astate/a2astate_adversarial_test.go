package a2astate

import "testing"

// TestDecodeRejectsMalformedState keeps a corrupt or truncated state from being
// read as a valid one: the decoder must degrade to an empty state, never to a
// half-populated state that a caller would act on.
func TestDecodeRejectsMalformedState(t *testing.T) {
	for _, raw := range []string{"", "   ", "{", "not json", `{"task_id":}`, "null", "[]"} {
		state := Decode(raw)
		if state != (State{}) {
			t.Fatalf("malformed state %q decoded to %+v, want the zero state", raw, state)
		}
	}
	// TaskID tolerates a raw identifier that is not JSON at all; what it must
	// never do is invent one out of whitespace.
	if id := TaskID("   "); id != "" {
		t.Fatalf("a blank state must not yield a task id, got %q", id)
	}
	if id := TaskID("  raw-task-id  "); id != "raw-task-id" {
		t.Fatalf("a raw identifier must be trimmed and preserved, got %q", id)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	original := State{TaskID: "task-1"}
	encoded := Encode(original)
	if encoded == "" {
		t.Fatal("a populated state must encode to something")
	}
	if decoded := Decode(encoded); decoded.TaskID != original.TaskID {
		t.Fatalf("round trip changed the state: %+v", decoded)
	}
	if TaskID(encoded) != original.TaskID {
		t.Fatalf("TaskID did not read the round-tripped state")
	}
}
