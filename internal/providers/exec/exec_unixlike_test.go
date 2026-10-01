//go:build linux || darwin

package exec

import (
	"bytes"
	"context"
	"io"
	"os"
	goexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestExec_Success(t *testing.T) {
	p := NewProvider()
	out, err := p.Exec(middleware.CommandSpec{Runner: "echo", Args: []string{"hello"}})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !bytes.Contains(out, []byte("hello")) {
		t.Errorf("output should contain 'hello', got: %s", out)
	}
}

func TestExec_Failure(t *testing.T) {
	p := NewProvider()
	_, err := p.Exec(middleware.CommandSpec{Runner: "false"})
	if err == nil {
		t.Error("expected error for non-zero exit")
	}
}

func TestExec_WithDir(t *testing.T) {
	tmpDir := t.TempDir()
	markerPath := filepath.Join(tmpDir, "marker.txt")
	if err := os.WriteFile(markerPath, []byte("cwd-ok"), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewProvider()
	out, err := p.Exec(middleware.CommandSpec{Runner: "cat", Args: []string{"marker.txt"}, Dir: tmpDir})
	if err != nil {
		t.Fatalf("Exec with Dir: %v", err)
	}
	if !bytes.Contains(out, []byte("cwd-ok")) {
		t.Errorf("should read marker.txt via Dir, got: %s", out)
	}
}

func TestExec_WithEnv(t *testing.T) {
	p := NewProvider()
	out, err := p.Exec(middleware.CommandSpec{
		Runner: "sh",
		Args:   []string{"-c", "echo $MY_TEST_VAR"},
		Env:    []string{"MY_TEST_VAR=from_env"},
	})
	if err != nil {
		t.Fatalf("Exec with Env: %v", err)
	}
	if !bytes.Contains(out, []byte("from_env")) {
		t.Errorf("should see env var, got: %s", out)
	}
}

func TestExecSeparate_Success(t *testing.T) {
	p := NewProvider()
	result, err := p.ExecSeparate(context.Background(), middleware.CommandSpec{
		Runner: "sh",
		Args:   []string{"-c", "echo out; echo err >&2"},
	})
	if err != nil {
		t.Fatalf("ExecSeparate: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exitCode = %d, want 0", result.ExitCode)
	}
	if !bytes.Contains(result.Stdout, []byte("out")) {
		t.Errorf("stdout should contain 'out', got: %s", result.Stdout)
	}
	if !bytes.Contains(result.Stderr, []byte("err")) {
		t.Errorf("stderr should contain 'err', got: %s", result.Stderr)
	}
}

func TestExecSeparate_NonzeroExit(t *testing.T) {
	p := NewProvider()
	result, err := p.ExecSeparate(context.Background(), middleware.CommandSpec{
		Runner: "sh",
		Args:   []string{"-c", "echo fail >&2; exit 42"},
	})
	if err != nil {
		t.Fatalf("ExecSeparate should not error on nonzero exit: %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("exitCode = %d, want 42", result.ExitCode)
	}
	if !bytes.Contains(result.Stderr, []byte("fail")) {
		t.Errorf("stderr should contain 'fail', got: %s", result.Stderr)
	}
}

func TestExecSeparate_WithDir(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "dir_test.txt"), []byte("dir-works"), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewProvider()
	result, err := p.ExecSeparate(context.Background(), middleware.CommandSpec{
		Runner: "cat",
		Args:   []string{"dir_test.txt"},
		Dir:    tmpDir,
	})
	if err != nil {
		t.Fatalf("ExecSeparate with Dir: %v", err)
	}
	if !bytes.Contains(result.Stdout, []byte("dir-works")) {
		t.Errorf("should read file via Dir, got: %s", result.Stdout)
	}
}

func TestExecSeparate_ContextCancel(t *testing.T) {
	p := NewProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := p.ExecSeparate(ctx, middleware.CommandSpec{
		Runner: "sleep",
		Args:   []string{"10"},
	})
	// ExecSeparate returns a result with nonzero exit code on context cancellation,
	// not an error — the process is killed and the exit code reflects that.
	if err != nil {
		// Some systems return an error directly
		return
	}
	if result.ExitCode == 0 {
		t.Error("expected nonzero exit code from cancelled context")
	}
}

func TestStart_Success(t *testing.T) {
	p := NewProvider()
	handle, err := p.Start(middleware.CommandSpec{Runner: "sleep", Args: []string{"60"}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = handle.Kill() }()

	if handle.GetPID() <= 0 {
		t.Errorf("PID should be positive, got %d", handle.GetPID())
	}

	if err := handle.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}
}

func TestStart_InvalidCommand(t *testing.T) {
	p := NewProvider()
	_, err := p.Start(middleware.CommandSpec{Runner: "nonexistent_command_xyz"})
	if err == nil {
		t.Error("expected error for nonexistent command")
	}
}

func TestStartPiped_Success(t *testing.T) {
	p := NewProvider()
	pp, err := p.StartPiped(middleware.CommandSpec{
		Runner: "sh",
		Args:   []string{"-c", "echo piped-output; echo piped-err >&2"},
	})
	if err != nil {
		t.Fatalf("StartPiped: %v", err)
	}

	out, err := io.ReadAll(pp.Stdout())
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Contains(out, []byte("piped-output")) {
		t.Errorf("should contain stdout, got: %s", out)
	}
	if !bytes.Contains(out, []byte("piped-err")) {
		t.Errorf("should contain stderr, got: %s", out)
	}
}

func TestStartPiped_WithDir(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "piped_test.txt"), []byte("piped-dir"), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewProvider()
	pp, err := p.StartPiped(middleware.CommandSpec{
		Runner: "cat",
		Args:   []string{"piped_test.txt"},
		Dir:    tmpDir,
	})
	if err != nil {
		t.Fatalf("StartPiped with Dir: %v", err)
	}

	out, err := io.ReadAll(pp.Stdout())
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Contains(out, []byte("piped-dir")) {
		t.Errorf("should read file via Dir, got: %s", out)
	}
}

func TestSpawnPTY_NotImplemented(t *testing.T) {
	p := NewProvider()
	err := p.SpawnPTY()
	if err == nil {
		t.Error("expected error for SpawnPTY")
	}
}

func TestHasExecutable_True(t *testing.T) {
	p := NewProvider()
	if !p.HasExecutable("echo") {
		t.Error("echo should be found in PATH")
	}
}

func TestHasExecutable_False(t *testing.T) {
	p := NewProvider()
	if p.HasExecutable("nonexistent_binary_xyz_12345") {
		t.Error("nonexistent binary should not be found")
	}
}

// isolatedEnvHome builds a home directory whose nvm initialization puts one
// shared directory on PATH, and puts a binary for every requested name in that
// directory. It does not run or install a real nvm: the point is that the
// environment to source exists and resolves arbitrary names, exactly as an
// installed toolchain does. The PATH entry is expanded by the lookup shell when
// it sources the file, mirroring how nvm.sh prepends its own bin directory.
func isolatedEnvHome(t *testing.T, names ...string) string {
	t.Helper()

	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		path := filepath.Join(bin, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	nvmDir := filepath.Join(home, ".nvm")
	if err := os.MkdirAll(nvmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "export PATH='" + bin + "':\"$PATH\"\n"
	if err := os.WriteFile(filepath.Join(nvmDir, "nvm.sh"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// resolutionOracle answers the same question HasExecutable answers, from the
// outside and without calling it: a name is resolvable when it is in $PATH now,
// or when the shell environment in this home puts a directory containing it on
// PATH. It reads the environment file the way the contract describes it instead
// of trusting the implementation, so it can disagree with a name-based answer.
func resolutionOracle(t *testing.T, home, name string) bool {
	t.Helper()

	if _, err := goexec.LookPath(name); err == nil {
		return true
	}
	body, err := os.ReadFile(filepath.Join(home, ".nvm", "nvm.sh"))
	if err != nil {
		return false // nothing to source: only $PATH can answer
	}
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "export PATH=") {
			continue
		}
		// The assignment is a shell word, so its parts can be quoted
		// independently: each PATH entry is unquoted on its own, and an entry
		// that is still a variable reference names no directory to check.
		value := strings.TrimPrefix(line, "export PATH=")
		for _, entry := range strings.Split(value, ":") {
			entry = strings.TrimSuffix(strings.TrimPrefix(entry, "'"), "'")
			entry = strings.TrimSuffix(strings.TrimPrefix(entry, `"`), `"`)
			if entry == "" || !strings.HasPrefix(entry, "/") || strings.ContainsAny(entry, "$`") {
				continue
			}
			if info, err := os.Stat(filepath.Join(entry, name)); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return true
			}
		}
	}
	return false
}

// TestHasExecutableIsNameAgnostic is the agnosticism contract for executable
// resolution: the answer comes from the environment that can resolve the name,
// never from the name itself.
//
// Each case asks about a name the retired allowlist did NOT contain (mimo,
// kimi), because the defect was exactly that listed and unlisted names got
// different answers. The expectation is computed by resolutionOracle, not by
// the implementation: it says the same binary is resolvable under one name and
// absent under another in the very same home, so a name-based answer is caught
// whichever way it is written.
func TestHasExecutableIsNameAgnostic(t *testing.T) {
	p := NewProvider()
	for _, test := range []struct {
		why    string
		home   string
		absent []string
		found  []string
	}{
		{
			why:    "no shell environment to source",
			home:   t.TempDir(),
			absent: []string{"node", "mimo", "kimi", "nonexistent_binary_xyz_12345"},
		},
		{
			why:    "shell environment resolves names the retired allowlist never had",
			home:   isolatedEnvHome(t, "mimo"),
			absent: []string{"kimi", "nonexistent_binary_xyz_12345"},
			found:  []string{"mimo"},
		},
		{
			why:    "shell environment resolves a listed name too",
			home:   isolatedEnvHome(t, "node"),
			absent: []string{"mimo"},
			found:  []string{"node"},
		},
	} {
		t.Run(test.why, func(t *testing.T) {
			// The provider and the oracle read the same HOME, so the comparison
			// is about the name, not about the host this runs on.
			t.Setenv("HOME", test.home)

			for _, name := range append(append([]string{}, test.found...), test.absent...) {
				want := resolutionOracle(t, test.home, name)
				if got := p.HasExecutable(name); got != want {
					t.Fatalf("HasExecutable(%q) = %v, oracle says %v (HOME=%s): the answer must come from the environment, not from the name", name, got, want, test.home)
				}
			}
			// Guard the oracle itself: without this, an oracle that always says
			// false would make the assertions above meaningless.
			if len(test.found) > 0 && !resolutionOracle(t, test.home, test.found[0]) {
				t.Fatalf("oracle cannot see %q in %s although the test built it there", test.found[0], test.home)
			}
		})
	}
}

// TestResolutionOracleMatchesASourcingShell anchors the oracle to the real
// thing: in the current home, the names a shell that sources the environment
// can resolve must be the names the oracle reports. Without this, the oracle
// above could be a second opinion that is simply wrong in the same way.
func TestResolutionOracleMatchesASourcingShell(t *testing.T) {
	home := os.Getenv("HOME")
	p := NewProvider()
	for _, name := range []string{"node", "mimo", "nonexistent_binary_xyz_12345"} {
		cmd := goexec.Command("bash", "-c", `export NVM_DIR="$HOME/.nvm"; if [ -s "$NVM_DIR/nvm.sh" ]; then \. "$NVM_DIR/nvm.sh"; fi; which "$1"`, "bash", name)
		sourced := cmd.Run() == nil
		if want := resolutionOracle(t, home, name); want != sourced {
			t.Fatalf("resolutionOracle(%q) = %v but a sourcing shell says %v: the oracle does not describe the real environment", name, want, sourced)
		}
		if want := p.HasExecutable(name); want != sourced {
			t.Fatalf("HasExecutable(%q) = %v but a sourcing shell says %v", name, want, sourced)
		}
	}
}
