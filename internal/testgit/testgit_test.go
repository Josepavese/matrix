package testgit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnvironmentDropsWhatTheCallerInherited pins the isolation itself: an
// inherited GIT_DIR chooses which repository a command acts on, and an inherited
// GIT_CONFIG_GLOBAL chooses the configuration that decides whether it can commit
// at all, so neither may survive into the environment this package builds.
func TestEnvironmentDropsWhatTheCallerInherited(t *testing.T) {
	t.Setenv("GIT_DIR", "/nonexistent/inherited")
	t.Setenv("GIT_CONFIG_GLOBAL", "/nonexistent/inherited-config")
	env := Environment(t)
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "GIT_DIR=") {
		t.Fatalf("an inherited GIT_DIR survived into the environment: %v", env)
	}
	if strings.Contains(joined, "/nonexistent/inherited-config") {
		t.Fatalf("the inherited configuration file is still the one in use: %v", env)
	}
	for _, required := range []string{"GIT_CONFIG_GLOBAL=", "GIT_CONFIG_SYSTEM=", "GIT_TERMINAL_PROMPT=0", "GIT_AUTHOR_NAME="} {
		if !strings.Contains(joined, required) {
			t.Fatalf("the environment does not set %s: %v", required, env)
		}
	}
	for _, entry := range env {
		if !strings.HasPrefix(entry, "GIT_CONFIG_GLOBAL=") {
			continue
		}
		path := strings.TrimPrefix(entry, "GIT_CONFIG_GLOBAL=")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("the configuration file this environment points at does not exist: %v", err)
		}
		if info.Size() != 0 {
			t.Fatalf("the configuration file is not empty: %d bytes", info.Size())
		}
	}
}

// TestCommandCommitsUnderAHostileCallerEnvironment is the end-to-end half: the
// caller's configuration asks for signed commits with no key in sight and points
// GIT_DIR at a repository that does not exist, and the repository below is still
// created and committed, because the command does not read either.
func TestCommandCommitsUnderAHostileCallerEnvironment(t *testing.T) {
	hostile := filepath.Join(t.TempDir(), "hostile-gitconfig")
	if err := os.WriteFile(hostile, []byte("[commit]\n\tgpgsign = true\n[core]\n\thooksPath = /nonexistent/hooks\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", hostile)
	t.Setenv("GIT_DIR", "/nonexistent/inherited")
	repo := filepath.Join(t.TempDir(), "repo")
	Run(t, "init", "-q", repo)
	Run(t, "-C", repo, "commit", "-q", "--allow-empty", "-m", "seed")
	if got := Output(t, "-C", repo, "log", "-1", "--format=%s"); got != "seed" {
		t.Fatalf("the commit was not recorded: %q", got)
	}
}
