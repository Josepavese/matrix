package semanticfs

import (
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

func TestRejectTraversalOversizedIDsAndUnboundedListings(t *testing.T) {
	view := New(Source{Storage: memstore.New(), Agents: func() ([]AgentView, error) { return []AgentView{{ID: "one"}, {ID: "two"}}, nil }, MaxEntries: 1})
	for _, name := range []string{"../config/private", "/config/private", "runs/../config/private", "runs\\..\\config", "config/private"} {
		if _, err := view.Open(name); err == nil {
			t.Fatal("unsafe namespace accepted", name)
		}
	}
	if _, err := DirectoryName(strings.Repeat("x", 126)); err == nil {
		t.Fatal("oversized identifier silently truncated")
	}
	if _, err := view.Open("agents"); err == nil {
		t.Fatal("listing budget ignored")
	}
}
