package workspacegrant

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

func gitCommand(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func TestGrantCoversOnlyOwnedRepositoryAndSelectedWorktrees(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	worktree := filepath.Join(t.TempDir(), "linked")
	foreign := filepath.Join(t.TempDir(), "foreign")
	for _, path := range []string{root, foreign} {
		gitCommand(t, "init", "-q", path)
	}
	gitCommand(t, "-C", root, "-c", "user.name=Matrix", "-c", "user.email=matrix@example.invalid", "commit", "-q", "--allow-empty", "-m", "seed")
	gitCommand(t, "-C", root, "worktree", "add", "-q", "-b", "test-linked", worktree)
	store := NewStore(memstore.New())
	ctx := context.Background()
	grant, err := store.Register(ctx, root, false, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authorize(ctx, root); err != nil {
		t.Fatalf("registered root refused: %v", err)
	}
	if _, err := store.Authorize(ctx, worktree); err == nil {
		t.Fatal("linked worktree accepted without opt-in")
	}
	grant, err = store.Register(ctx, root, true, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authorize(ctx, worktree); err != nil {
		t.Fatalf("linked worktree refused: %v", err)
	}
	later := filepath.Join(t.TempDir(), "later-linked")
	gitCommand(t, "-C", root, "worktree", "add", "-q", "-b", "test-later", later)
	if _, err := store.Authorize(ctx, later); err != nil {
		t.Fatalf("new linked worktree refused: %v", err)
	}
	for _, path := range []string{foreign, filepath.Dir(root), filepath.Join(root, "missing")} {
		if _, err := store.Authorize(ctx, path); err == nil {
			t.Fatalf("unrelated or invalid path accepted: %s", path)
		}
	}
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(worktree, link); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authorize(ctx, link); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink accepted or misclassified: %v", err)
	}
	if err := store.Revoke(grant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authorize(ctx, worktree); err == nil {
		t.Fatal("revoked grant accepted")
	}
}

func TestGrantRejectsExpiredAndInvalidRepository(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	gitCommand(t, "init", "-q", root)
	storage := memstore.New()
	store := NewStore(storage)
	ctx := context.Background()
	if _, err := store.Register(ctx, "relative", true, time.Hour); err == nil {
		t.Fatal("relative repository path accepted")
	}
	if _, err := store.Register(ctx, root, true, time.Second); err == nil {
		t.Fatal("too-short grant accepted")
	}
	grant, err := store.Register(ctx, root, true, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	grant.ExpiresAt = time.Now().Add(-time.Second)
	encoded, err := json.Marshal(grant)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Set(keyPrefix+grant.ID, encoded); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authorize(ctx, root); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired grant accepted: %v", err)
	}
}
