package semanticfs

import (
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

func TestSelectedViewsExcludeSecretsAndOpenFilesAreSnapshots(t *testing.T) {
	store := memstore.New()
	_ = store.Set("config.private", []byte("PRIVATE_CONFIG"))
	runs := runtrace.NewStore(store)
	run, _, err := runs.Start(runtrace.Run{ID: "run-proof", AgentID: "agent", ClientMeta: map[string]interface{}{"secret": "PRIVATE_CLIENT"}})
	if err != nil {
		t.Fatal(err)
	}
	view := New(Source{Storage: store})
	dir, _ := DirectoryName(run.ID)
	path := "runs/" + dir + "/status.json"
	opened, err := view.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if _, err := runs.Complete(run.ID, "PRIVATE_SUMMARY", "end_turn"); err != nil {
		t.Fatal(err)
	}
	old, _ := io.ReadAll(opened)
	fresh, err := fs.ReadFile(view, path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(old), `"status": "running"`) || !strings.Contains(string(fresh), `"status": "completed"`) {
		t.Fatal("open snapshot or live view broken")
	}
	if strings.Contains(string(fresh), "PRIVATE_") {
		t.Fatal("private fields projected")
	}
	if err := fstest.TestFS(view, "runs/"+dir+"/status.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFile(view, "runs/"+dir+"/summary.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("summary exposed without opt-in", err)
	}
	summaryView := New(Source{Storage: store, IncludeSummaries: true})
	data, err := fs.ReadFile(summaryView, "runs/"+dir+"/summary.txt")
	if err != nil || string(data) != "PRIVATE_SUMMARY" {
		t.Fatal("opt-in summary unavailable", err)
	}
	for _, name := range []string{"../config/private", "/config/private", "runs/../config/private", "runs\\..\\config", "config/private"} {
		if _, err := view.Open(name); err == nil {
			t.Fatal("unsafe namespace accepted", name)
		}
	}
}

func TestPortableDirectoryNamesDistinguishCaseAndReservedWindowsNames(t *testing.T) {
	seen := map[string]bool{}
	for _, id := range []string{"mimo", "MIMO", "CON", "con", "trailing. ", "a/b\\c:question?"} {
		name, err := DirectoryName(id)
		if err != nil {
			t.Fatal(err)
		}
		folded := strings.ToLower(name)
		if seen[folded] {
			t.Fatal("case-insensitive collision")
		}
		seen[folded] = true
		decoded, err := decodeID(name)
		if err != nil || decoded != id {
			t.Fatal("ID did not round trip")
		}
	}
}
