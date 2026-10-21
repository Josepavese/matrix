package runpayload

import (
	"encoding/json"
	"testing"
)

// TestUnmarshalJSONRejectsUnusablePayloads keeps a malformed request from being
// silently read as an empty turn: an empty turn would look like a successful run
// with no output, which is the hardest failure to notice.
func TestUnmarshalJSONRejectsUnusablePayloads(t *testing.T) {
	for _, raw := range []string{"null", "{", "not json", "[]", "123", `{"text":42}`, `{"other":"x"}`} {
		var input Input
		if err := json.Unmarshal([]byte(raw), &input); err == nil {
			t.Fatalf("payload %q must be rejected, got %q", raw, input.String())
		}
	}
}

func TestUnmarshalJSONAcceptsBothDocumentedSpellings(t *testing.T) {
	var plain Input
	if err := json.Unmarshal([]byte(`"hello"`), &plain); err != nil {
		t.Fatalf("a bare string must be accepted: %v", err)
	}
	if plain.String() != "hello" {
		t.Fatalf("bare string decoded to %q", plain.String())
	}
	var structured Input
	if err := json.Unmarshal([]byte(`{"text":"hello"}`), &structured); err != nil {
		t.Fatalf("a text object must be accepted: %v", err)
	}
	if structured.String() != "hello" {
		t.Fatalf("object decoded to %q", structured.String())
	}
}
