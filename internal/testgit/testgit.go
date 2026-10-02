// Package testgit runs git from a test with a configuration the test owns.
//
// A test that drives git must not read the configuration of the machine running
// it. An ambient commit.gpgsign, core.hooksPath or init.templateDir decides
// whether its commits exist at all, and an ambient GIT_DIR outranks the -C that
// chooses which repository a command acts on. When a test inherits either, it
// passes or fails depending on who runs it, which is a property of the machine
// rather than of the code under test.
//
// The package exists because that isolation was written once per test package,
// and four copies of a rule are four chances for it to drift. It imports
// testing for the same reason net/http/httptest does: it is test support that
// ships as an ordinary package, and no production binary imports it.
package testgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Environment returns the environment for a git command run by a test: every
// inherited GIT_* variable is dropped, and the two configuration files are empty
// files created for this test. The identity is set here as well, so a command
// does not need -c user.name/-c user.email to commit.
func Environment(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	global := filepath.Join(dir, "gitconfig-global")
	system := filepath.Join(dir, "gitconfig-system")
	for _, path := range []string{global, system} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	env := make([]string, 0, len(os.Environ())+10)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "GIT_") {
			continue
		}
		env = append(env, entry)
	}
	return append(env,
		"GIT_CONFIG_GLOBAL="+global,
		"GIT_CONFIG_SYSTEM="+system,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"GIT_AUTHOR_NAME=Matrix",
		"GIT_AUTHOR_EMAIL=matrix@example.invalid",
		"GIT_COMMITTER_NAME=Matrix",
		"GIT_COMMITTER_EMAIL=matrix@example.invalid",
	)
}

// Command returns a git command whose environment comes from Environment.
func Command(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = Environment(t)
	return cmd
}

// Output runs git and returns its trimmed output, failing the test on a non-zero
// exit.
func Output(t *testing.T, args ...string) string {
	t.Helper()
	out, err := Command(t, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// Run runs git and fails the test on a non-zero exit.
func Run(t *testing.T, args ...string) {
	t.Helper()
	_ = Output(t, args...)
}
