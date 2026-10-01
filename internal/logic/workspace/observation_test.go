package workspace

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestWorkspaceObservationCarriesBothSides is the artifact contract: the run
// reports what was asked for and what it resolved to, on the same record.
func TestWorkspaceObservationCarriesBothSides(t *testing.T) {
	comparison, err := CompareObservations(
		Observation{WorkspaceID: "ws-worktree", Path: "/work/root"},
		Observation{WorkspaceID: "ws-worktree", Path: "/work/root", RealChildCwd: "/work/root"},
	)
	if err != nil {
		t.Fatalf("an agreeing workspace must not be refused: %v", err)
	}
	if comparison.Requested.WorkspaceID != "ws-worktree" || comparison.Resolved.WorkspaceID != "ws-worktree" {
		t.Fatalf("both sides must travel together: %+v", comparison)
	}
	if comparison.Requested.Path != "/work/root" || comparison.Resolved.Path != "/work/root" {
		t.Fatalf("both paths must travel together: %+v", comparison)
	}
}

// TestWorkspaceObservationNamesTheDisagreeingField is fail-closed: two sides that
// denote different workspaces refuse the run and name the field, so the caller can
// fix the hint instead of reading a summary of a run in the wrong directory.
func TestWorkspaceObservationNamesTheDisagreeingField(t *testing.T) {
	for name, testCase := range map[string]struct {
		requested Observation
		resolved  Observation
		field     string
	}{
		"workspace_id": {
			requested: Observation{WorkspaceID: "ws-asked", Path: "/work/root"},
			resolved:  Observation{WorkspaceID: "ws-other", Path: "/work/root"},
			field:     "workspace_id",
		},
		"path": {
			requested: Observation{WorkspaceID: "ws-worktree", Path: "/work/asked"},
			resolved:  Observation{WorkspaceID: "ws-worktree", Path: "/work/other"},
			field:     "path",
		},
		"branch": {
			requested: Observation{Path: "/work/root", Branch: "feature"},
			resolved:  Observation{Path: "/work/root", Branch: "main"},
			field:     "branch",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := CompareObservations(testCase.requested, testCase.resolved)
			if err == nil {
				t.Fatalf("%s disagreement must refuse the run", name)
			}
			var mismatch *ComparisonError
			if !errors.As(err, &mismatch) {
				t.Fatalf("refusal must be typed, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), testCase.field) {
				t.Fatalf("refusal must name %q, got %q", testCase.field, err)
			}
		})
	}
}

// TestWorkspaceObservationRefusesAChildOutsideTheWorkspace covers the case the
// run must never survive: the process runs in a directory that is not the
// workspace the run was bound to, which is how workstreams contaminate each other.
func TestWorkspaceObservationRefusesAChildOutsideTheWorkspace(t *testing.T) {
	_, err := CompareObservations(
		Observation{WorkspaceID: "ws-worktree", Path: "/work/worktree"},
		Observation{WorkspaceID: "ws-worktree", Path: "/work/worktree", RealChildCwd: "/work/root"},
	)
	if err == nil {
		t.Fatal("a child running outside the run workspace must be refused")
	}
	if !strings.Contains(err.Error(), "real_child_cwd") {
		t.Fatalf("refusal must name the child directory, got %q", err)
	}
}

// TestWorkspaceObservationTreatsAnAbsentHintAsUnknown keeps a caller who named
// only an id from being refused for not repeating the path, and keeps an
// unregistered path from pretending to be an id.
func TestWorkspaceObservationTreatsAnAbsentHintAsUnknown(t *testing.T) {
	if _, err := CompareObservations(
		Observation{WorkspaceID: "ws-worktree"},
		Observation{WorkspaceID: "ws-worktree", Path: "/work/worktree", RealChildCwd: "/work/worktree"},
	); err != nil {
		t.Fatalf("an absent hint is unknown, not a disagreement: %v", err)
	}
	if _, err := CompareObservations(
		Observation{Path: "/work/free"},
		Observation{Path: "/work/free", RealChildCwd: "/work/free"},
	); err != nil {
		t.Fatalf("a path with no registered id is a normal resolution: %v", err)
	}
}

// TestWorkspaceObservationComparesPathsAsDirectories mirrors the identity
// resolution: two spellings of one directory are one directory, so a trailing
// slash or a relative segment must not fail a run.
func TestWorkspaceObservationComparesPathsAsDirectories(t *testing.T) {
	root := filepath.Join("/work", "root")
	if _, err := CompareObservations(
		Observation{Path: root + "/"},
		Observation{Path: filepath.Join("/work", ".", "root"), RealChildCwd: root},
	); err != nil {
		t.Fatalf("one directory spelled twice must agree: %v", err)
	}
}

// TestWorkspaceObservationDeclaresWhatItDidNotDerive is the honesty clause: the
// repository fields are absent because Matrix did not look, and the artifact says
// so instead of leaving a reader to read absence as agreement.
func TestWorkspaceObservationDeclaresWhatItDidNotDerive(t *testing.T) {
	comparison, err := CompareObservations(
		Observation{WorkspaceID: "ws-worktree", Path: "/work/worktree"},
		Observation{WorkspaceID: "ws-worktree", Path: "/work/worktree", RealChildCwd: "/work/worktree"},
	)
	if err != nil {
		t.Fatalf("an agreeing workspace must not be refused: %v", err)
	}
	for _, field := range []string{"git_common_dir", "branch"} {
		reason, found := comparison.NotDerived[field]
		if !found {
			t.Fatalf("%s must be declared as not derived, got %+v", field, comparison.NotDerived)
		}
		if !strings.Contains(reason, "does not interrogate") {
			t.Fatalf("%s must carry the reason, got %q", field, reason)
		}
	}
	if comparison.Resolved.GitCommonDir != "" || comparison.Resolved.Branch != "" {
		t.Fatalf("a value Matrix did not derive must not be invented: %+v", comparison.Resolved)
	}
	if _, found := comparison.NotDerived["real_child_cwd"]; found {
		t.Fatalf("a child directory the transport reported is derived, not declared: %+v", comparison.NotDerived)
	}
}

// TestWorkspaceObservationReportsTheChildDirectoryWhenItIsKnown covers the
// resolved side of the tuple once the transport has started the child: the
// artifact reports the directory the process really runs in.
func TestWorkspaceObservationReportsTheChildDirectoryWhenItIsKnown(t *testing.T) {
	comparison, err := CompareObservations(
		Observation{WorkspaceID: "ws-worktree", Path: "/work/worktree"},
		Observation{WorkspaceID: "ws-worktree", Path: "/work/worktree", RealChildCwd: "/work/worktree"},
	)
	if err != nil {
		t.Fatalf("an agreeing workspace must not be refused: %v", err)
	}
	if comparison.Resolved.RealChildCwd != "/work/worktree" {
		t.Fatalf("the resolved child directory must be reported, got %+v", comparison.Resolved)
	}
}
