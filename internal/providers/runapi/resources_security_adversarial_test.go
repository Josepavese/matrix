package runapi

import (
	"strings"
	"testing"
)

// TestSSEEventNameCannotForgeFields keeps the event name a single field value: it
// is written into a line-oriented protocol, so a newline would let it inject
// additional fields or whole events into the stream.
func TestSSEEventNameCannotForgeFields(t *testing.T) {
	hostile := "evil\ndata: injected\n\nid: 1"
	got := sseEventName(hostile)
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("the event name must be a single line, got %q", got)
	}
	if got == "" {
		t.Fatal("an event name must never be empty")
	}
	if strings.TrimSpace(got) == "" {
		t.Fatalf("a whitespace-only name must fall back, got %q", got)
	}
	if sseEventName("") != "message" {
		t.Fatalf("an empty kind must fall back to a usable name, got %q", sseEventName(""))
	}
	if sseEventName("tool.call.requested") != "tool.call.requested" {
		t.Fatalf("a normal kind must pass through unchanged, got %q", sseEventName("tool.call.requested"))
	}
}
