package sessionview

import (
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestRenderActionReturnsAnExplicitMessageUnchanged keeps a decision that was
// already explained from being re-rendered into something else: the caller's
// message is the answer.
func TestRenderActionReturnsAnExplicitMessageUnchanged(t *testing.T) {
	// A "list" action with a message is the one exception: it renders the list.
	result := middleware.SessionActionResult{Action: "activate", Message: "Sessione attivata."}
	if got := RenderAction(result, "it", RenderDeps{}); got != "Sessione attivata." {
		t.Fatalf("an explicit message must be returned unchanged, got %q", got)
	}
}

// TestRenderActionNeverPanicsOnDegenerateInput is the safety property: an
// unknown action, a missing session and a missing lookup must all degrade
// gracefully rather than panic in a chat handler. An unknown action with no
// message renders empty by contract: the manager only produces known actions,
// and the caller's message stays authoritative.
func TestRenderActionNeverPanicsOnDegenerateInput(t *testing.T) {
	cases := map[string]middleware.SessionActionResult{
		"unknown action":         {Action: "nonsense"},
		"status without session": {Action: "status"},
		"empty result":           {},
		"list without sessions":  {Action: "list"},
	}
	for name, result := range cases {
		t.Run(name, func(_ *testing.T) {
			_ = RenderAction(result, "it", RenderDeps{Lookup: func(string) string { return "" }})
		})
	}
	// A nil lookup is the sharpest case: the status renderer calls it when the
	// session is missing.
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("a nil lookup must not panic: %v", recovered)
			}
		}()
		_ = RenderAction(middleware.SessionActionResult{Action: "status"}, "it", RenderDeps{})
	}()
}

// TestRenderStatusNamesTheSessionAndItsWorkspace pins the useful part of the
// status line: without the identifiers an operator cannot act on it.
func TestRenderStatusNamesTheSessionAndItsWorkspace(t *testing.T) {
	lookup := func(key string) string {
		if key == "session_status" {
			return "sess=%s details=%s agent=%s created=%s"
		}
		return "missing"
	}
	result := middleware.SessionActionResult{
		Action:  "status",
		Session: &middleware.SessionEntry{LogicalSessionID: "s1", AgentID: "codex", Alias: "lavoro"},
	}
	rendered := RenderAction(result, "it", RenderDeps{Lookup: lookup})
	for _, fragment := range []string{"sess=s1", "agent=codex", "Alias: \"lavoro\""} {
		if !contains(rendered, fragment) {
			t.Fatalf("status %q is missing %q", rendered, fragment)
		}
	}

	// A missing session must say so instead of rendering empty fields.
	notFound := RenderAction(middleware.SessionActionResult{Action: "status"}, "it", RenderDeps{
		Lookup: func(key string) string {
			if key == "session_not_found_db" {
				return "sessione non trovata"
			}
			return ""
		},
	})
	if notFound != "sessione non trovata" {
		t.Fatalf("a missing session must report lookup text, got %q", notFound)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
