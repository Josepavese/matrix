//go:build linux || darwin

package exec

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
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
