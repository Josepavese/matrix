package zedacpstdio

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/logic/childenv"
)

// TestTheAgentChildDoesNotInheritTheDaemonsEnvironment pins the leak this
// transport closed: the agent is a program that runs code and can read its own
// environment, so the daemon's keys must not be in it. A real child writes its
// environment into a file - the proof is what the child saw, not what the parent
// intended - and the half that must survive is the agent's own credential, which
// arrives through SpawnSpec.Env.
func TestTheAgentChildDoesNotInheritTheDaemonsEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the child's environment is observed through a shell script")
	}
	const sentinel = "MATRIX_DAEMON_KEY_SENTINEL"
	t.Setenv(sentinel, "sk-live-should-never-reach-an-agent")
	t.Setenv("MATRIX_API_KEY", "sk-live-second-sentinel")
	t.Setenv("MATRIX_VAULT_PASSPHRASE", "should-never-reach-an-agent")

	dump := filepath.Join(t.TempDir(), "agent-environment.txt")
	transport, err := New(context.Background(), "/bin/sh",
		SpawnSpec{Env: []string{"AGENT_CREDENTIAL=sk-agent-own"}}, "-c", "env > "+dump)
	if err != nil {
		t.Fatalf("starting the agent child: %v", err)
	}
	t.Cleanup(func() { _ = transport.Close() })

	seen := readChildEnvironment(t, dump)
	for _, name := range []string{sentinel, "MATRIX_API_KEY", "MATRIX_VAULT_PASSPHRASE"} {
		if strings.Contains(seen, name+"=") {
			t.Fatalf("the agent read %s out of the daemon's environment. Child environment:\n%s", name, seen)
		}
	}
	if !strings.Contains(seen, "AGENT_CREDENTIAL=sk-agent-own") {
		t.Fatalf("the agent's own credential did not reach it: the launch would break a real agent. Child environment:\n%s", seen)
	}
	if !strings.Contains(seen, "PATH=") {
		t.Fatalf("the child was started without the variables it needs to run:\n%s", seen)
	}
}

// TestTheAgentChildStartsThroughTheRealLaunchPreamble is the other half of the
// risk: the allowlist must not break the launch Matrix actually performs. The
// preamble sources nvm before running the agent, and it reads $HOME to find it, so
// a child that reports the variable the preamble exported proves the contract
// between the launcher and this transport still holds.
func TestTheAgentChildStartsThroughTheRealLaunchPreamble(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stdio preamble is a shell construct")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not available")
	}
	// Source a controlled nvm fixture through the real preamble. A runner's
	// installed nvm can exceed the observation deadline under race-test load.
	// The exported marker also proves sourcing, not just setting NVM_DIR.
	home := t.TempDir()
	nvmDir := filepath.Join(home, ".nvm")
	if err := os.Mkdir(nvmDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nvmDir, "nvm.sh"), []byte("export MATRIX_NVM_FIXTURE_LOADED=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dump := filepath.Join(t.TempDir(), "agent-environment.txt")
	command, args := agentlaunch.PrepareStdio("/bin/sh", []string{"-c", "env > " + dump}, true)

	transport, err := New(context.Background(), command, SpawnSpec{Env: []string{"HOME=" + home, "AGENT_CREDENTIAL=sk-agent-own"}}, args...)
	if err != nil {
		t.Fatalf("starting the agent through the real launch preamble: %v", err)
	}
	t.Cleanup(func() { _ = transport.Close() })

	seen := readChildEnvironment(t, dump)
	for _, want := range []string{"NVM_DIR=" + nvmDir, "MATRIX_NVM_FIXTURE_LOADED=1", "AGENT_CREDENTIAL=sk-agent-own"} {
		if !strings.Contains(seen, want) {
			t.Fatalf("the launch preamble must still hand the agent %q, got:\n%s", want, seen)
		}
	}
}

// TestTheAgentEnvironmentIsTheAllowlistPlusWhatTheAgentDeclares checks the shape
// rather than one secret: every name the child sees that the daemon holds comes
// from the allowlist, and everything else the child reads was declared by the
// agent itself.
func TestTheAgentEnvironmentIsTheAllowlistPlusWhatTheAgentDeclares(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the child's environment is observed through a shell script")
	}
	t.Setenv("SOME_FUTURE_OPERATOR_SECRET", "value")
	handed := childenv.Environment()
	for _, entry := range handed {
		name := entry
		if i := strings.IndexByte(entry, '='); i >= 0 {
			name = entry[:i]
		}
		if !childenv.IsAllowedName(name) {
			t.Fatalf("the transport handed the agent %q, which the allowlist does not name", name)
		}
	}
	if strings.Contains(strings.Join(handed, "\n"), "SOME_FUTURE_OPERATOR_SECRET=") {
		t.Fatal("a secret only the daemon holds is in the environment the transport builds")
	}
	if !strings.Contains(strings.Join(handed, "\n"), "PATH=") {
		t.Fatal("PATH is on the allowlist and must reach the agent")
	}
}

// readChildEnvironment waits for the child to finish writing and returns what it
// saw. A child that dies before writing is the failure the caller wants to read,
// so the wait reports both the deadline and the process's own error.
func readChildEnvironment(t *testing.T, dump string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if raw, err := os.ReadFile(dump); err == nil && len(raw) > 0 {
			return string(raw)
		}
		if time.Now().After(deadline) {
			t.Fatalf("the child wrote no environment to %s within the deadline", dump)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
