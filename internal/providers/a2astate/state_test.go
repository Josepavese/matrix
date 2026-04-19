package a2astate

import "testing"

func TestEncodeDecodeAndTaskID(t *testing.T) {
	encoded := Encode(State{TaskID: "task-1", ContextID: "ctx-1"})
	decoded := Decode(encoded)
	if decoded.TaskID != "task-1" || decoded.ContextID != "ctx-1" {
		t.Fatalf("unexpected decoded state: %+v", decoded)
	}
	if got := TaskID(encoded); got != "task-1" {
		t.Fatalf("unexpected task id from encoded state: %q", got)
	}
	if got := TaskID("task-raw"); got != "task-raw" {
		t.Fatalf("unexpected raw task id: %q", got)
	}
}
