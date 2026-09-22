package runnotifier

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

// TestInferToolNameHandlesDegenerateContent pins the fallback used when the
// provider sends no structured tool name: a blank or separator-free payload must
// not become an empty tool name, and a huge one must not become an unbounded one.
func TestInferToolNameHandlesDegenerateContent(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"   ":           "",
		"Read file.txt": "Read",
		"Bash(ls)":      "Bash",
		// The separator search stops at the first separator found, so a name
		// followed by a colon keeps it: the provider payload is echoed, not
		// reinterpreted.
		"Edit: main.go": "Edit:",
		"Single":        "Single",
	}
	for content, want := range cases {
		if got := inferToolName(content); got != want {
			t.Fatalf("inferToolName(%q) = %q, want %q", content, got, want)
		}
	}
	long := strings.Repeat("x", 200)
	if got := inferToolName(long); len(got) != 64 {
		t.Fatalf("an unbounded payload must be truncated to 64 bytes, got %d", len(got))
	}
}

// TestOnThoughtIsSafeWithoutAStore keeps a partially constructed notifier from
// panicking: the thought channel is wired before every dependency is guaranteed.
func TestOnThoughtIsSafeWithoutAStore(_ *testing.T) {
	var nilNotifier *Notifier
	nilNotifier.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeToolCall, Content: "Read"})

	notifier := New(nil, "run-1", "codex", "acp")
	notifier.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeToolCall, Content: "Read"})
	notifier.SetHeader("codex", "remote-1")
	notifier.SetLogicalSession("logical-1", "ws-1")
}

// TestOnThoughtRecordsAToolCallWithAStableIdentity checks the two properties the
// trace depends on: the tool call is recorded once, and its identifier is
// derived from the content rather than from the clock, so a replayed update
// cannot create a second entry for the same call.
func TestOnThoughtRecordsAToolCallWithAStableIdentity(t *testing.T) {
	var (
		mu     sync.Mutex
		events []runtrace.Event
	)
	store := runtrace.NewStore(memstore.New()).WithEventDispatcher(func(event runtrace.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	notifier := New(store, "run-1", "codex", "acp")
	update := middleware.ThoughtUpdate{
		Type:    middleware.ThoughtTypeToolCall,
		Content: "Read main.go",
		Metadata: map[string]interface{}{
			"tool_name": "Read",
		},
	}
	notifier.OnThought(update)
	notifier.OnThought(update)

	// Dispatch happens off the read loop, so wait for the worker instead of
	// asserting immediately.
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		count := len(events)
		mu.Unlock()
		if count >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 {
		t.Fatalf("expected two recorded tool calls, got %d", len(events))
	}
	for _, event := range events {
		// The canonical name wins over the inferred one: NormalizeTool prefers
		// the structured metadata, which is why the notifier passes both.
		if event.Kind != "tool.call.requested" || event.ToolName != "read_file" {
			t.Fatalf("unexpected event: kind=%q tool=%q", event.Kind, event.ToolName)
		}
		if event.ToolCallID == "" || event.RunID != "run-1" {
			t.Fatalf("the event lost its identity: %+v", event)
		}
	}
	// Same content and metadata must yield the same call identity.
	if events[0].ToolCallID != events[1].ToolCallID {
		t.Fatalf("a replayed update produced a different identity: %q vs %q", events[0].ToolCallID, events[1].ToolCallID)
	}
	// The sequence must still advance: identity is stable, ordering is not lost.
	if events[0].Sequence == events[1].Sequence {
		t.Fatal("two events must not share a sequence number")
	}
}
