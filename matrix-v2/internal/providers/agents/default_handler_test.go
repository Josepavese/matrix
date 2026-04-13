package agents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jose/matrix-v2/internal/providers/exec"
	"github.com/jose/matrix-v2/internal/providers/osfs"
	"strings"
)

func TestHandleTerminalCreate_Echo(t *testing.T) {
	handler := newConfigurableRequestHandler(nil).
		WithProcess(exec.NewProvider())

	params, _ := json.Marshal(map[string]interface{}{
		"command": "echo",
		"args":    []string{"hello", "world"},
	})

	result, err := handler.HandleRequest(context.Background(), "terminal/create", params)
	if err != nil {
		t.Fatalf("HandleRequest terminal/create: %v", err)
	}

	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}

	exitCode, ok := m["exitCode"].(int)
	if !ok {
		t.Fatalf("exitCode type = %T, want int", m["exitCode"])
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
	stdout, ok := m["stdout"].(string)
	if !ok {
		t.Fatalf("stdout type = %T, want string", m["stdout"])
	}
	if stdout == "" {
		t.Error("stdout should not be empty")
	}
}

func TestHandleTerminalCreate_InvalidJSON(t *testing.T) {
	handler := newConfigurableRequestHandler(nil).
		WithProcess(exec.NewProvider())

	_, err := handler.HandleRequest(context.Background(), "terminal/create", json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestHandleTerminalCreate_MissingCommand(t *testing.T) {
	handler := newConfigurableRequestHandler(nil).
		WithProcess(exec.NewProvider())

	params, _ := json.Marshal(map[string]interface{}{
		"args": []string{"foo"},
	})

	_, err := handler.HandleRequest(context.Background(), "terminal/create", params)
	if err == nil {
		t.Error("expected error for missing command")
	}
}

func TestHandleTerminalCreate_NoProcess(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)

	params, _ := json.Marshal(map[string]interface{}{
		"command": "echo",
		"args":    []string{"test"},
	})

	_, err := handler.HandleRequest(context.Background(), "terminal/create", params)
	if err == nil {
		t.Error("expected error when process provider is nil")
	}
}

func TestHandleTerminalCreate_NonzeroExit(t *testing.T) {
	handler := newConfigurableRequestHandler(nil).
		WithProcess(exec.NewProvider())

	params, _ := json.Marshal(map[string]interface{}{
		"command": "sh",
		"args":    []string{"-c", "echo err >&2; exit 42"},
	})

	result, err := handler.HandleRequest(context.Background(), "terminal/create", params)
	if err != nil {
		t.Fatalf("HandleRequest should not error for nonzero exit: %v", err)
	}

	m := result.(map[string]interface{})
	exitCode, _ := m["exitCode"].(int)
	if exitCode != 42 {
		t.Errorf("exitCode = %d, want 42", exitCode)
	}
	stderr, _ := m["stderr"].(string)
	if stderr == "" {
		t.Error("stderr should contain 'err'")
	}
}

func TestHandleTerminalCreate_CwdPropagation(t *testing.T) {
	tmpDir := t.TempDir()
	markerFile := "cwd_marker.txt"
	markerPath := filepath.Join(tmpDir, markerFile)
	os.WriteFile(markerPath, []byte("found"), 0644)

	handler := newConfigurableRequestHandler(nil).
		WithProcess(exec.NewProvider()).
		WithFS(osfs.NewFSProvider(), tmpDir)

	// Command: cat a file using a relative path — should resolve against tmpDir
	params, _ := json.Marshal(map[string]interface{}{
		"command": "cat",
		"args":    []string{markerFile},
	})

	result, err := handler.HandleRequest(context.Background(), "terminal/create", params)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	m := result.(map[string]interface{})
	exitCode, _ := m["exitCode"].(int)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr: %s", exitCode, m["stderr"])
	}

	stdout := m["stdout"].(string)
	if !strings.Contains(stdout, "found") {
		t.Errorf("stdout should contain 'found', got: %q — command ran in wrong directory", stdout)
	}
}

func TestHandleTerminalCreate_CwdFromRequest(t *testing.T) {
	outerDir := t.TempDir()
	innerDir := filepath.Join(outerDir, "sub")
	os.MkdirAll(innerDir, 0755)
	os.WriteFile(filepath.Join(innerDir, "inner.txt"), []byte("inner-content"), 0644)

	handler := newConfigurableRequestHandler(nil).
		WithProcess(exec.NewProvider()).
		WithFS(osfs.NewFSProvider(), outerDir)

	params, _ := json.Marshal(map[string]interface{}{
		"command": "cat",
		"args":    []string{"inner.txt"},
		"cwd":     "sub",
	})

	result, err := handler.HandleRequest(context.Background(), "terminal/create", params)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	m := result.(map[string]interface{})
	stdout := m["stdout"].(string)
	if !strings.Contains(stdout, "inner-content") {
		t.Errorf("stdout should contain 'inner-content', got: %q", stdout)
	}
}

func TestHandleFSRead_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello fs"), 0644)

	handler := newConfigurableRequestHandler(nil).
		WithFS(osfs.NewFSProvider(), tmpDir)

	params, _ := json.Marshal(map[string]interface{}{
		"path": "test.txt",
	})

	result, err := handler.HandleRequest(context.Background(), "fs/read_text_file", params)
	if err != nil {
		t.Fatalf("fs/read_text_file: %v", err)
	}

	m := result.(map[string]interface{})
	if m["content"].(string) != "hello fs" {
		t.Errorf("content = %q, want 'hello fs'", m["content"])
	}
}

func TestHandleFSWrite_Success(t *testing.T) {
	tmpDir := t.TempDir()

	handler := newConfigurableRequestHandler(nil).
		WithFS(osfs.NewFSProvider(), tmpDir)

	params, _ := json.Marshal(map[string]interface{}{
		"path":    "sub/dir/output.txt",
		"content": "written by agent",
	})

	result, err := handler.HandleRequest(context.Background(), "fs/write_text_file", params)
	if err != nil {
		t.Fatalf("fs/write_text_file: %v", err)
	}

	m := result.(map[string]interface{})
	if m["status"].(string) != "ok" {
		t.Errorf("status = %v, want ok", m["status"])
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "sub", "dir", "output.txt"))
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if string(data) != "written by agent" {
		t.Errorf("file content = %q, want 'written by agent'", string(data))
	}
}

func TestHandleFSRead_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	handler := newConfigurableRequestHandler(nil).
		WithFS(osfs.NewFSProvider(), tmpDir)

	params, _ := json.Marshal(map[string]interface{}{
		"path": "../../../etc/passwd",
	})

	_, err := handler.HandleRequest(context.Background(), "fs/read_text_file", params)
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestHandleFSRead_NoFS(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)

	params, _ := json.Marshal(map[string]interface{}{
		"path": "test.txt",
	})

	_, err := handler.HandleRequest(context.Background(), "fs/read_text_file", params)
	if err == nil {
		t.Error("expected error when fs is nil")
	}
}

func TestResolvePath_BoundaryCases(t *testing.T) {
	tmpDir := filepath.Clean(t.TempDir())
	handler := newConfigurableRequestHandler(nil).WithFS(nil, tmpDir)

	tests := []struct {
		name   string
		input  string
		empty  bool
	}{
		{"empty string", "", true},
		{"simple file", "foo.txt", false},
		{"subdirectory", "sub/bar.txt", false},
		{"dot", ".", false},
		{"double dot parent", "..", true},
		{"deep traversal", "a/../../../etc/passwd", true},
		{"absolute within cwd", tmpDir + "/safe.txt", false},
		{"absolute outside cwd is sandboxed to cwd", "/etc/passwd", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := handler.resolvePath(tc.input)
			if tc.empty && result != "" {
				t.Errorf("resolvePath(%q) = %q, want empty", tc.input, result)
			}
			if !tc.empty && result == "" {
				t.Errorf("resolvePath(%q) = empty, want non-empty", tc.input)
			}
			if !tc.empty && result != "" {
				// Verify resolved path is within cwd
				if result != tmpDir && !(len(result) > len(tmpDir) && result[:len(tmpDir)+1] == tmpDir+"/") {
					t.Errorf("resolvePath(%q) = %q, not within cwd %s", tc.input, result, tmpDir)
				}
			}
		})
	}
}

func TestPermissionRequest_TrustMode(t *testing.T) {
	trusted := true
	handler := newConfigurableRequestHandler(func() bool { return trusted })

	params, _ := json.Marshal(map[string]interface{}{
		"options": []map[string]interface{}{
			{"optionId": "opt1", "kind": "allow_once"},
		},
	})

	// Trust mode on → auto-approve
	result, err := handler.HandleRequest(context.Background(), "session/request_permission", params)
	if err != nil {
		t.Fatalf("permission request: %v", err)
	}
	m := result.(map[string]interface{})
	outcome := m["outcome"].(map[string]interface{})
	if outcome["outcome"].(string) != "selected" {
		t.Errorf("expected selected outcome, got %v", outcome["outcome"])
	}

	// Trust mode off → deny
	trusted = false
	result, err = handler.HandleRequest(context.Background(), "session/request_permission", params)
	if err != nil {
		t.Fatalf("permission request (denied): %v", err)
	}
	m = result.(map[string]interface{})
	outcome = m["outcome"].(map[string]interface{})
	if outcome["outcome"].(string) != "denied" {
		t.Errorf("expected denied outcome, got %v", outcome["outcome"])
	}
}

func TestTerminalMethods_Stub(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)

	for _, method := range []string{"terminal/output", "terminal/wait_for_exit", "terminal/kill", "terminal/release"} {
		result, err := handler.HandleRequest(context.Background(), method, json.RawMessage(`{}`))
		if err != nil {
			t.Errorf("%s: unexpected error: %v", method, err)
		}
		m := result.(map[string]interface{})
		if m["status"].(string) != "not_implemented" {
			t.Errorf("%s: expected not_implemented, got %v", method, m["status"])
		}
	}
}

func TestHandleRequest_DefaultAutoApprove(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)

	result, err := handler.HandleRequest(context.Background(), "some/unknown/method", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unknown method: %v", err)
	}
	m := result.(map[string]interface{})
	if m["status"].(string) != "ok" {
		t.Errorf("unknown method should auto-approve, got %v", m["status"])
	}
}
