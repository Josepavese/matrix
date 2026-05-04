package agents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

type recordingNotifier struct {
	updates []middleware.ThoughtUpdate
}

func (n *recordingNotifier) OnThought(update middleware.ThoughtUpdate) {
	n.updates = append(n.updates, update)
}

func (n *recordingNotifier) SetHeader(_, _ string) {}

func (n *recordingNotifier) FormattedHeader() string { return "" }

func startTerminalForTest(t *testing.T, handler *defaultRequestHandler, params json.RawMessage) string {
	t.Helper()
	result, err := handler.HandleRequest(context.Background(), "terminal/create", params)
	if err != nil {
		t.Fatalf("terminal/create: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	terminalID, ok := m["terminalId"].(string)
	if !ok || terminalID == "" {
		t.Fatalf("expected terminalId in result, got %#v", m)
	}
	return terminalID
}

func waitTerminalForTest(t *testing.T, handler *defaultRequestHandler, terminalID string) map[string]interface{} {
	t.Helper()
	params, _ := json.Marshal(map[string]interface{}{"terminalId": terminalID})
	result, err := handler.HandleRequest(context.Background(), "terminal/wait_for_exit", params)
	if err != nil {
		t.Fatalf("terminal/wait_for_exit: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map wait result, got %T", result)
	}
	return m
}

func outputTerminalForTest(t *testing.T, handler *defaultRequestHandler, terminalID string) map[string]interface{} {
	t.Helper()
	params, _ := json.Marshal(map[string]interface{}{"terminalId": terminalID})
	result, err := handler.HandleRequest(context.Background(), "terminal/output", params)
	if err != nil {
		t.Fatalf("terminal/output: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map output result, got %T", result)
	}
	return m
}

func TestHandleTerminalCreate_Echo(t *testing.T) {
	handler := newConfigurableRequestHandler(nil).
		WithProcess(exec.NewProvider())

	params, _ := json.Marshal(map[string]interface{}{
		"command": "echo",
		"args":    []string{"hello", "world"},
	})

	terminalID := startTerminalForTest(t, handler, params)
	wait := waitTerminalForTest(t, handler, terminalID)
	exitCode, ok := wait["exitCode"].(int)
	if !ok {
		t.Fatalf("exitCode type = %T, want int", wait["exitCode"])
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
	output := outputTerminalForTest(t, handler, terminalID)
	stdout, ok := output["output"].(string)
	if !ok {
		t.Fatalf("output type = %T, want string", output["output"])
	}
	if stdout == "" {
		t.Error("output should not be empty")
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

	terminalID := startTerminalForTest(t, handler, params)
	wait := waitTerminalForTest(t, handler, terminalID)
	exitCode, _ := wait["exitCode"].(int)
	if exitCode != 42 {
		t.Errorf("exitCode = %d, want 42", exitCode)
	}
	output := outputTerminalForTest(t, handler, terminalID)
	text, _ := output["output"].(string)
	if !strings.Contains(text, "err") {
		t.Errorf("output should contain 'err', got %q", text)
	}
}

func TestHandleTerminalCreate_CwdPropagation(t *testing.T) {
	tmpDir := t.TempDir()
	markerFile := "cwd_marker.txt"
	markerPath := filepath.Join(tmpDir, markerFile)
	if err := os.WriteFile(markerPath, []byte("found"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := newConfigurableRequestHandler(nil).
		WithProcess(exec.NewProvider()).
		WithFS(osfs.NewFSProvider(), tmpDir)

	// Command: cat a file using a relative path — should resolve against tmpDir
	params, _ := json.Marshal(map[string]interface{}{
		"command": "cat",
		"args":    []string{markerFile},
	})

	terminalID := startTerminalForTest(t, handler, params)
	wait := waitTerminalForTest(t, handler, terminalID)
	exitCode, _ := wait["exitCode"].(int)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}

	output := outputTerminalForTest(t, handler, terminalID)
	stdout, ok := output["output"].(string)
	if !ok {
		t.Fatal("output is not a string")
	}
	if !strings.Contains(stdout, "found") {
		t.Errorf("output should contain 'found', got: %q — command ran in wrong directory", stdout)
	}
}

func TestHandleTerminalCreate_CwdFromRequest(t *testing.T) {
	outerDir := t.TempDir()
	innerDir := filepath.Join(outerDir, "sub")
	if err := os.MkdirAll(innerDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(innerDir, "inner.txt"), []byte("inner-content"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := newConfigurableRequestHandler(nil).
		WithProcess(exec.NewProvider()).
		WithFS(osfs.NewFSProvider(), outerDir)

	params, _ := json.Marshal(map[string]interface{}{
		"command": "cat",
		"args":    []string{"inner.txt"},
		"cwd":     "sub",
	})

	terminalID := startTerminalForTest(t, handler, params)
	wait := waitTerminalForTest(t, handler, terminalID)
	exitCode, _ := wait["exitCode"].(int)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	output := outputTerminalForTest(t, handler, terminalID)
	stdout, ok := output["output"].(string)
	if !ok {
		t.Fatal("output is not a string")
	}
	if !strings.Contains(stdout, "inner-content") {
		t.Errorf("output should contain 'inner-content', got: %q", stdout)
	}
}

func TestHandleFSRead_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello fs"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := newConfigurableRequestHandler(nil).
		WithFS(osfs.NewFSProvider(), tmpDir)

	params, _ := json.Marshal(map[string]interface{}{
		"path": "test.txt",
	})

	result, err := handler.HandleRequest(context.Background(), "fs/read_text_file", params)
	if err != nil {
		t.Fatalf("fs/read_text_file: %v", err)
	}

	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not a map, got %T", result)
	}
	content, _ := m["content"].(string)
	if content != "hello fs" {
		t.Errorf("content = %q, want 'hello fs'", m["content"])
	}
}

func TestHandleFSRead_LineAndLimit(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("one\ntwo\nthree\nfour\n"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := newConfigurableRequestHandler(nil).
		WithFS(osfs.NewFSProvider(), tmpDir)

	params, _ := json.Marshal(map[string]interface{}{
		"path":  filepath.Join(tmpDir, "test.txt"),
		"line":  2,
		"limit": 2,
	})

	result, err := handler.HandleRequest(context.Background(), "fs/read_text_file", params)
	if err != nil {
		t.Fatalf("fs/read_text_file: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not a map, got %T", result)
	}
	if m["content"] != "two\nthree\n" {
		t.Fatalf("unexpected sliced content: %#v", m["content"])
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

	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not a map, got %T", result)
	}
	status, _ := m["status"].(string)
	if status != "ok" {
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

func TestHandleFSWrite_EmitsStructuralToolEvents(t *testing.T) {
	tmpDir := t.TempDir()
	notifier := &recordingNotifier{}
	handler := newConfigurableRequestHandler(nil).
		WithFS(osfs.NewFSProvider(), tmpDir).
		WithNotifier(notifier)

	params, _ := json.Marshal(map[string]interface{}{
		"path":    "sub/dir/output.txt",
		"content": "sensitive file content must not enter metadata",
	})
	if _, err := handler.HandleRequest(context.Background(), "fs/write_text_file", params); err != nil {
		t.Fatalf("fs/write_text_file: %v", err)
	}

	if len(notifier.updates) != 2 {
		t.Fatalf("expected request and result tool updates, got %#v", notifier.updates)
	}
	if notifier.updates[0].Type != middleware.ThoughtTypeToolCall || notifier.updates[1].Type != middleware.ThoughtTypeToolResult {
		t.Fatalf("unexpected update types: %#v", notifier.updates)
	}
	meta := notifier.updates[0].Metadata
	if meta["protocol_method"] != "fs/write_text_file" || meta["tool_kind"] != "edit" || meta["tool_name"] != "write_file" {
		t.Fatalf("unexpected request metadata: %#v", meta)
	}
	path, ok := meta["path"].(string)
	if !ok || !strings.HasSuffix(path, "sub/dir/output.txt") {
		t.Fatalf("expected resolved path in metadata, got %#v", meta["path"])
	}
	raw, ok := meta["raw_input"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected sanitized raw_input, got %#v", meta["raw_input"])
	}
	if _, leaked := raw["content"]; leaked {
		t.Fatalf("raw_input leaked file content: %#v", raw)
	}
	if notifier.updates[1].Metadata["status"] != "completed" {
		t.Fatalf("expected completed result metadata, got %#v", notifier.updates[1].Metadata)
	}
}

func TestHandleTerminalCreate_EmitsStructuralToolEvents(t *testing.T) {
	tmpDir := t.TempDir()
	notifier := &recordingNotifier{}
	handler := newConfigurableRequestHandler(nil).
		WithProcess(exec.NewProvider()).
		WithFS(osfs.NewFSProvider(), tmpDir).
		WithNotifier(notifier)

	params, _ := json.Marshal(map[string]interface{}{
		"command": "sh",
		"args":    []string{"-c", "exit 42"},
	})
	terminalID := startTerminalForTest(t, handler, params)
	wait := waitTerminalForTest(t, handler, terminalID)
	if wait["exitCode"] != 42 {
		t.Fatalf("expected nonzero command result, got %#v", wait)
	}

	if len(notifier.updates) != 2 {
		t.Fatalf("expected request and result tool updates, got %#v", notifier.updates)
	}
	if notifier.updates[0].Metadata["tool_kind"] != "execute" || notifier.updates[0].Metadata["protocol_method"] != "terminal/create" {
		t.Fatalf("unexpected request metadata: %#v", notifier.updates[0].Metadata)
	}
	if notifier.updates[1].Metadata["status"] != "failed" || notifier.updates[1].Metadata["exit_code"] != 42 {
		t.Fatalf("expected failed result metadata, got %#v", notifier.updates[1].Metadata)
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
		name  string
		input string
		empty bool
	}{
		{"empty string", "", true},
		{"simple file", "foo.txt", false},
		{"subdirectory", "sub/bar.txt", false},
		{"dot", ".", false},
		{"double dot parent", "..", true},
		{"deep traversal", "a/../../../etc/passwd", true},
		{"absolute within cwd", tmpDir + "/safe.txt", false},
		{"absolute outside cwd rejected", "/etc/passwd", true},
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
				if result != tmpDir && (len(result) <= len(tmpDir) || result[:len(tmpDir)+1] != tmpDir+"/") {
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
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not a map, got %T", result)
	}
	outcome, _ := m["outcome"].(map[string]interface{})
	outcomeStr, _ := outcome["outcome"].(string)
	if outcomeStr != "selected" {
		t.Errorf("expected selected outcome, got %v", outcome["outcome"])
	}

	// Trust mode off → deny
	trusted = false
	result, err = handler.HandleRequest(context.Background(), "session/request_permission", params)
	if err != nil {
		t.Fatalf("permission request (denied): %v", err)
	}
	m, ok = result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not a map: %T", result)
	}
	outcome, _ = m["outcome"].(map[string]interface{})
	outcomeStr2, _ := outcome["outcome"].(string)
	if outcomeStr2 != "denied" {
		t.Errorf("expected denied outcome, got %v", outcome["outcome"])
	}
}

func TestTerminalMethods_MissingTerminalID(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)

	for _, method := range []string{"terminal/output", "terminal/wait_for_exit", "terminal/kill", "terminal/release"} {
		_, err := handler.HandleRequest(context.Background(), method, json.RawMessage(`{}`))
		if err == nil {
			t.Errorf("%s: expected error for missing terminalId", method)
		}
	}
}

func TestTerminalMethods_UnknownTerminal(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)

	for _, method := range []string{"terminal/output", "terminal/wait_for_exit", "terminal/kill", "terminal/release"} {
		_, err := handler.HandleRequest(context.Background(), method, json.RawMessage(`{"terminalId":"nonexistent"}`))
		if err == nil {
			t.Errorf("%s: expected error for unknown terminalId", method)
		}
	}
}

func TestHandleRequest_DefaultAutoApprove(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)

	result, err := handler.HandleRequest(context.Background(), "some/unknown/method", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unknown method: %v", err)
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not a map, got %T", result)
	}
	status, _ := m["status"].(string)
	if status != "ok" {
		t.Errorf("unknown method should auto-approve, got %v", m["status"])
	}
}
