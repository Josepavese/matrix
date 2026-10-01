package workspacegrant

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

// TestTheGitProbeHandsTheChildOnlyTheAllowlistedEnvironment observes the probe's
// child directly: a stand-in git records the environment it was started with, and
// the daemon sitting on GIT_DIR, GIT_WORK_TREE and GIT_COMMON_DIR must not appear
// in that record. The verdict is a security decision, so the proof is about what
// the child saw rather than about what the parent intended.
func TestTheGitProbeHandsTheChildOnlyTheAllowlistedEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the child's environment is observed through a shell script")
	}
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	fakeBin := t.TempDir()
	dump := filepath.Join(t.TempDir(), "child-environment.txt")
	script := fmt.Sprintf("#!/bin/sh\nenv > %s\nprintf '%%s\\n' %s %s\n", dump, resolvedRoot, filepath.Join(resolvedRoot, ".git"))
	if err := os.WriteFile(filepath.Join(fakeBin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	// What the daemon's environment says must not become what the child sees.
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "elsewhere", ".git"))
	t.Setenv("GIT_WORK_TREE", filepath.Join(t.TempDir(), "elsewhere"))
	t.Setenv("GIT_COMMON_DIR", filepath.Join(t.TempDir(), "elsewhere"))

	if _, err := NewStore(memstore.New()).Register(context.Background(), root, false, time.Hour); err != nil {
		t.Fatalf("the probe did not answer from the repository: %v", err)
	}
	seen, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("the child left no record of its environment: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(seen)), "\n") {
		if strings.HasPrefix(line, "GIT_") {
			t.Fatalf("the child saw %q: the verdict must not depend on the daemon's environment. Child environment:\n%s", line, seen)
		}
	}
	if !strings.Contains(string(seen), "PATH=") {
		t.Fatalf("the child was started without the variables it needs to run:\n%s", seen)
	}
}

// TestTheGitProbeVerdictDoesNotFollowTheDaemonsEnvironment is the consequence:
// with the daemon sitting on GIT_DIR, GIT_COMMON_DIR and GIT_WORK_TREE, a grant
// for one repository must still cover that repository, and must not cover a
// worktree of the repository the environment named.
//
// What each variable did before the probe had its own environment is worth
// stating exactly, because the milder reading of it is wrong. GIT_DIR and
// GIT_COMMON_DIR left the root check passing - git keeps the worktree at the
// current directory and still answers repoA - and moved the common directory a
// grant is keyed by to the other repository, so the run produced two greens where
// one had to be red: a grant for repoA covered a worktree of repoB. Only
// GIT_WORK_TREE was the fail-closed branch, answering a root that is not the
// registered one. "Fail-closed" is a conclusion about one variable, not a
// property of the tool.
func TestTheGitProbeVerdictDoesNotFollowTheDaemonsEnvironment(t *testing.T) {
	repoA := filepath.Join(t.TempDir(), "repoA")
	repoB := filepath.Join(t.TempDir(), "repoB")
	linkedB := filepath.Join(t.TempDir(), "linkedB")
	gitCommand(t, "init", "-q", repoA)
	gitCommand(t, "init", "-q", repoB)
	gitCommand(t, "-C", repoB, "-c", "user.name=Matrix", "-c", "user.email=matrix@example.invalid", "commit", "-q", "--allow-empty", "-m", "seed")
	gitCommand(t, "-C", repoB, "worktree", "add", "-q", "-b", "test-sentinel", linkedB)

	otherCommonDir := filepath.Join(repoB, ".git")
	store := NewStore(memstore.New())
	ctx := context.Background()

	t.Run("GIT_DIR cannot choose the repository a grant is keyed by", func(t *testing.T) {
		t.Setenv("GIT_DIR", otherCommonDir)
		grant, err := store.Register(ctx, repoA, true, time.Hour)
		if err != nil {
			t.Fatalf("a regular repository stopped being registrable with GIT_DIR=%s in the daemon's environment: %v", otherCommonDir, err)
		}
		if _, err := store.Authorize(ctx, repoA); err != nil {
			t.Fatalf("the grant stopped covering its own repository: %v", err)
		}
		if _, err := store.Authorize(ctx, linkedB); err == nil {
			t.Fatalf("a worktree of %s was authorized by a grant for %s: the child saw GIT_DIR=%s and answered with common directory %s",
				repoB, repoA, otherCommonDir, grant.CommonDir)
		}
	})

	t.Run("GIT_COMMON_DIR cannot choose the common directory a grant is keyed by", func(t *testing.T) {
		t.Setenv("GIT_COMMON_DIR", otherCommonDir)
		grant, err := store.Register(ctx, repoA, true, time.Hour)
		if err != nil {
			t.Fatalf("a regular repository stopped being registrable with GIT_COMMON_DIR=%s in the daemon's environment: %v", otherCommonDir, err)
		}
		if _, err := store.Authorize(ctx, repoA); err != nil {
			t.Fatalf("the grant stopped covering its own repository: %v", err)
		}
		if _, err := store.Authorize(ctx, linkedB); err == nil {
			t.Fatalf("a worktree of %s was authorized by a grant for %s: the child saw GIT_COMMON_DIR=%s and answered with common directory %s",
				repoB, repoA, otherCommonDir, grant.CommonDir)
		}
	})

	t.Run("GIT_WORK_TREE cannot move the root the grant is registered for", func(t *testing.T) {
		t.Setenv("GIT_WORK_TREE", repoB)
		if _, err := store.Register(ctx, repoA, false, time.Hour); err != nil {
			t.Fatalf("GIT_WORK_TREE=%s changed the verdict: the probe answered %v instead of reading %s", repoB, err, repoA)
		}
	})
}
