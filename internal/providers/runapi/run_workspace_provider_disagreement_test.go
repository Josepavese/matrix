package runapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/workspace"
	runresponse "github.com/Josepavese/matrix/internal/providers/runapi/response"
	"github.com/Josepavese/matrix/internal/testgit"
)

// sameDirectory compares two paths as directories: a temporary directory can be
// reached through a symlink (/tmp on macOS, /var on some hosts), and a test that
// compares the spellings would disagree with itself from one machine to the next.
func sameDirectory(t *testing.T, left, right string) bool {
	t.Helper()
	resolvedLeft, err := filepath.EvalSymlinks(left)
	if err != nil {
		t.Fatalf("resolve %s: %v", left, err)
	}
	resolvedRight, err := filepath.EvalSymlinks(right)
	if err != nil {
		t.Fatalf("resolve %s: %v", right, err)
	}
	return resolvedLeft == resolvedRight
}

// TestProviderCommittingOutsideTheResolvedWorkspaceIsNamedByTheArtifact is the
// EP-03.C demonstration, on the episode's own terms: run 54f552c5 was reported as
// "the provider committed on ROOT although the worktree was intended", and it was
// left open because the provider's logs were said to be needed.
//
// The logs are needed for the attribution - who told the provider where to work -
// and that stays out of reach, because Matrix does not hold them. The
// disagreement is a different question, and it does not need them: Matrix
// resolves the workspace, publishes it before the prompt, and declares in the same
// block which repository fields it did not interrogate. This test performs the
// provider's side for real - git commits in the root repository while the run's
// workspace is a linked worktree of that same repository - and shows that the
// artifact and the repository together name the disagreement, while nothing in
// the artifact claims to know where the child actually ran.
//
// Behavioural reverts this test catches, each applied to the tree, compiled, run,
// and then restored byte-identical (the same rows are in
// docs/governance/delivery-contract-reverts.md):
//   - run_workspace_resolution.go:121 drops workspace_requested from the
//     artifact: the two sides stop travelling together and the test fails on the
//     requested side being absent.
//   - run_workspace_resolution.go:123 drops workspace_not_derived: the artifact
//     stops declaring what it did not interrogate, and the test fails there.
//   - run_workspace_resolution.go:122 publishes the requested observation as the
//     resolved one: the artifact stops saying where the run was dispatched, and
//     the test fails on the resolved path being empty.
func TestProviderCommittingOutsideTheResolvedWorkspaceIsNamedByTheArtifact(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	worktree := filepath.Join(base, "worktree")
	git := func(args ...string) string {
		t.Helper()
		return testgit.Output(t, args...)
	}

	git("init", "-q", root)
	git("-C", root, "commit", "-q", "--allow-empty", "-m", "seed")
	git("-C", root, "worktree", "add", "-q", "-b", "intended", worktree)

	// The caller names the workspace by id, which the contract allows. The path
	// the run will use is therefore Matrix's own resolution, and the artifact is
	// the only place that publishes it.
	server := NewServer(&runTestRouter{}).WithTraceStorage(
		runWorkspaceStorage(t, workspace.Meta{ID: "ws-worktree", RootPath: worktree}))
	w := postRunRequest(t, server, `{"channel_id":"ep-03-c","input":"probe","workspace_id":"ws-worktree"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("run was not accepted: %d %s", w.Code, w.Body.String())
	}
	var success runresponse.Success
	if err := json.Unmarshal(w.Body.Bytes(), &success); err != nil {
		t.Fatalf("decode: %v", err)
	}

	event := findRunEvent(t, loadRunTrace(t, server, success.RunID), "workspace.identity.resolved")
	resolved, ok := event.Metadata["workspace_resolved"].(map[string]interface{})
	if !ok {
		t.Fatalf("the artifact does not publish the resolved workspace: %+v", event.Metadata)
	}
	dispatchedInto, ok := resolved["path"].(string)
	if !ok || dispatchedInto == "" {
		t.Fatalf("the artifact does not say where the run was dispatched: %+v", resolved)
	}
	if !sameDirectory(t, dispatchedInto, worktree) {
		t.Fatalf("the artifact points the run at %q, want the worktree it resolved %q", dispatchedInto, worktree)
	}
	requested, ok := event.Metadata["workspace_requested"].(map[string]interface{})
	if !ok {
		t.Fatalf("the artifact does not publish the requested workspace: %+v", event.Metadata)
	}
	if requested["workspace_id"] != "ws-worktree" {
		t.Fatalf("the requested side is not what the caller sent: %+v", requested)
	}
	if path, carriesPath := requested["path"]; carriesPath && path != "" {
		t.Fatalf("the artifact invents a requested path the caller never sent: %+v", requested)
	}

	// The same block declares what Matrix did not interrogate, so an absent
	// repository field cannot be read as an agreement, and nothing in the resolved
	// side claims a repository value Matrix never observed.
	notDerived, ok := event.Metadata["workspace_not_derived"].(map[string]interface{})
	if !ok {
		t.Fatalf("the artifact does not declare what it did not derive: %+v", event.Metadata)
	}
	for _, field := range []string{"git_common_dir", "branch", "real_child_cwd"} {
		reason, declared := notDerived[field].(string)
		if !declared || strings.TrimSpace(reason) == "" {
			t.Fatalf("%s must be declared as not derived, with its reason: %+v", field, notDerived)
		}
		if value, claimed := resolved[field]; claimed && value != nil && value != "" {
			t.Fatalf("the artifact claims %s=%v, which Matrix did not observe", field, value)
		}
	}

	// The provider's side, performed independently of the workspace the run was
	// given: it commits in the root repository, the way a provider running
	// `git -C <root> commit` does.
	git("-C", root, "commit", "-q", "--allow-empty", "-m", "provider wrote to ROOT")
	committedInto := git("-C", root, "rev-parse", "--show-toplevel")
	rootHead := git("-C", root, "rev-parse", "HEAD")
	if worktreeHead := git("-C", worktree, "rev-parse", "HEAD"); worktreeHead == rootHead {
		t.Fatalf("the provider's commit reached the run's workspace: this test needs it to land elsewhere")
	}
	if subject := git("-C", worktree, "log", "-1", "--format=%s"); subject == "provider wrote to ROOT" {
		t.Fatalf("the run's workspace carries the provider's commit")
	}

	// The disagreement, named, from two independent sources: where the artifact
	// records the dispatch, and the repository the provider's own git wrote to.
	if sameDirectory(t, committedInto, dispatchedInto) {
		t.Fatalf("this test needs the two sides to disagree, got %q on both", committedInto)
	}

	// And it names a fact, not a suspicion: the artifact was published before the
	// provider acted, so reading the trace again after the commit must show the
	// same recorded dispatch.
	afterEvent := findRunEvent(t, loadRunTrace(t, server, success.RunID), "workspace.identity.resolved")
	after, ok := afterEvent.Metadata["workspace_resolved"].(map[string]interface{})
	if !ok || after["path"] != dispatchedInto {
		t.Fatalf("the artifact changed after the provider acted: %v -> %+v", dispatchedInto, afterEvent.Metadata["workspace_resolved"])
	}

	// The limit, stated and checked: the directory the child actually ran in is
	// published by the transport that starts it, after this point, so no event in
	// this trace claims it. Deciding whether the provider was told to work in the
	// root, or chose to, needs the provider's logs, which Matrix does not hold;
	// the artifact is honest about what it did not see instead of guessing.
	for _, recorded := range loadRunTrace(t, server, success.RunID).Events {
		if value, claimed := recorded.Metadata["real_child_cwd"]; claimed && value != nil && value != "" {
			t.Fatalf("event %s claims the child's directory %v: the attribution would rest on a value Matrix did not observe", recorded.Kind, value)
		}
	}
}
