package runpayload

import (
	"encoding/json"
	"testing"
)

func TestInputUnmarshalStringAndStructuredText(t *testing.T) {
	var compact Input
	if err := json.Unmarshal([]byte(`"hello"`), &compact); err != nil {
		t.Fatalf("compact input failed: %v", err)
	}
	if compact.String() != "hello" {
		t.Fatalf("unexpected compact input: %q", compact.String())
	}

	var structured Input
	if err := json.Unmarshal([]byte(`{"text":"hello structured"}`), &structured); err != nil {
		t.Fatalf("structured input failed: %v", err)
	}
	if structured.String() != "hello structured" {
		t.Fatalf("unexpected structured input: %q", structured.String())
	}
}
