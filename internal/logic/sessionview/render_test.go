package sessionview

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestRenderStatusIncludesDetails(t *testing.T) {
	result := middleware.SessionActionResult{
		Action: "status",
		Session: &middleware.SessionEntry{
			LogicalSessionID: "session-1",
			AgentID:          "codex",
			Alias:            "main",
			WorkspaceID:      "matrix",
			WorkspacePath:    "/repo",
			Mode:             "review",
			CreatedAt:        "2026-04-24T08:00:00Z",
			LastHandoff: &middleware.HandoffPacket{
				FromAgentID: "codex",
				ToAgentID:   "opencode",
			},
		},
	}

	got := RenderAction(result, "en", testDeps())

	for _, want := range []string{
		"Session session-1",
		"Alias: \"main\"",
		"Workspace: matrix (/repo)",
		"Mode: review",
		"Handoff: codex -> opencode",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered status missing %q in %q", want, got)
		}
	}
}

func TestRenderListIncludesLocalAndRemoteSessions(t *testing.T) {
	result := middleware.SessionActionResult{
		Action: "list",
		Sessions: []middleware.SessionEntry{
			{LogicalSessionID: "local-1", AgentID: "codex"},
		},
		RemoteSessions: []middleware.RemoteSessionInfo{
			{RemoteSessionID: "remote-1", DisplayID: "r1"},
		},
	}

	got := RenderAction(result, "en", testDeps())

	for _, want := range []string{"History", "local:codex:local-1", "Remote sessions:", "remote:r1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered list missing %q in %q", want, got)
		}
	}
}

func testDeps() RenderDeps {
	return RenderDeps{
		Lookup: func(key string) string {
			switch key {
			case "session_not_found_db":
				return "not found"
			case "session_status":
				return "Session %s%s\nAgent: %s\nCreated: %s"
			case "session_history_header":
				return "History"
			default:
				return key
			}
		},
		Local: func(_ int, _ string, session middleware.SessionEntry) string {
			return "local:" + session.AgentID + ":" + session.LogicalSessionID + "\n"
		},
		Remote: func(_ int, session middleware.RemoteSessionInfo) string {
			return "remote:" + session.DisplayID + "\n"
		},
	}
}
