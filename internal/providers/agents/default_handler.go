package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jose/matrix-v2/internal/middleware"
)

// Compile-time check: middleware.File satisfies the write/close interface used in handleFSWrite.
var _ interface {
	Write([]byte) (int, error)
	Close() error
} = (middleware.File)(nil)

// defaultRequestHandler handles incoming JSON-RPC requests from agents.
// When trustMode is true (default), it auto-approves all permission requests.
// When trustMode is false, it denies permission requests (deny-by-default).
// It also handles fs/ and terminal/ ACP methods.
type defaultRequestHandler struct {
	trustMode func() bool        // returns true if auto-approve is enabled; defaults to true if nil
	fs        middleware.FS      // filesystem operations for fs/* methods
	proc      middleware.Process // process execution for terminal/* methods
	cwd       string             // working directory for path validation

	// terminalRegistry holds active terminal sessions for async terminal methods.
	terminals   map[string]*terminalSession
	terminalsMu sync.Mutex
}

// terminalSession tracks a running terminal process and its accumulated output.
type terminalSession struct {
	id     middleware.PipedProcess
	output bytes.Buffer
	done   chan struct{}
	once   sync.Once
}

// newConfigurableRequestHandler creates a handler that consults the given
// trustMode function for permission decisions and supports fs/terminal ACP methods.
func newConfigurableRequestHandler(trustMode func() bool) *defaultRequestHandler {
	return &defaultRequestHandler{trustMode: trustMode, terminals: make(map[string]*terminalSession)}
}

// NewDefaultRequestHandler exposes the configurable ACP request handler to protocol adapters.
func NewDefaultRequestHandler(trustMode func() bool) *defaultRequestHandler {
	return newConfigurableRequestHandler(trustMode)
}

// WithFS configures filesystem operations for fs/* ACP methods.
func (h *defaultRequestHandler) WithFS(fs middleware.FS, cwd string) *defaultRequestHandler {
	h.fs = fs
	h.cwd = cwd
	return h
}

// WithProcess configures process execution for terminal/* ACP methods.
func (h *defaultRequestHandler) WithProcess(proc middleware.Process) *defaultRequestHandler {
	h.proc = proc
	return h
}

func (h *defaultRequestHandler) isTrustMode() bool {
	if h.trustMode == nil {
		return true
	}
	return h.trustMode()
}

func (h *defaultRequestHandler) HandleRequest(ctx context.Context, method string, params json.RawMessage) (interface{}, error) {
	log := slog.With("component", "acp_request_handler", "method", method)
	log.Info("handling agent request", "event", "agent_request", "method", method, "params_len", len(params))

	switch method {
	case "session/request_permission":
		return h.handlePermissionRequest(ctx, log, params)
	case "fs/read_text_file":
		return h.handleFSRead(ctx, log, params)
	case "fs/write_text_file":
		return h.handleFSWrite(ctx, log, params)
	case "terminal/create":
		return h.handleTerminalCreate(ctx, log, params)
	case "terminal/output":
		return h.handleTerminalOutput(ctx, log, params)
	case "terminal/wait_for_exit":
		return h.handleTerminalWaitForExit(ctx, log, params)
	case "terminal/kill":
		return h.handleTerminalKill(ctx, log, params)
	case "terminal/release":
		return h.handleTerminalRelease(ctx, log, params)
	default:
		log.Info("auto-approving agent request", "event", "request_approved", "method", method)
		return map[string]interface{}{"status": "ok"}, nil
	}
}

// --- Permission handling ---

func (h *defaultRequestHandler) handlePermissionRequest(_ context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	var req struct {
		Options []struct {
			OptionID string `json:"optionId"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		log.Warn("failed to parse permission request", "error", err)
		if h.isTrustMode() {
			return h.approveResponse("allow-once"), nil
		}
		return h.denyResponse(), nil
	}

	if !h.isTrustMode() {
		log.Info("denying permission (trust mode off)", "event", "permission_denied", "options_count", len(req.Options))
		return h.denyResponse(), nil
	}

	optionID := "allow-once"
	for _, opt := range req.Options {
		if opt.Kind == "allow_once" || opt.Kind == "allow_always" {
			optionID = opt.OptionID
			break
		}
	}
	log.Info("auto-approving permission", "event", "permission_approved", "optionID", optionID, "options_count", len(req.Options))
	return h.approveResponse(optionID), nil
}

func (h *defaultRequestHandler) approveResponse(optionID string) map[string]interface{} {
	return map[string]interface{}{
		"outcome": map[string]interface{}{
			"outcome":  "selected",
			"optionId": optionID,
		},
	}
}

func (h *defaultRequestHandler) denyResponse() map[string]interface{} {
	return map[string]interface{}{
		"outcome": map[string]interface{}{
			"outcome": "denied",
		},
	}
}

// --- File system methods ---

func (h *defaultRequestHandler) handleFSRead(_ context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	if h.fs == nil {
		log.Warn("fs handler not configured")
		return nil, fmt.Errorf("filesystem access not available")
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid fs/read_text_file params: %w", err)
	}

	absPath := h.resolvePath(req.Path)
	if absPath == "" {
		return nil, fmt.Errorf("invalid path: %s", req.Path)
	}

	data, err := h.fs.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", absPath, err)
	}

	log.Info("agent read file", "path", absPath, "bytes", len(data))
	return map[string]interface{}{
		"content": string(data),
	}, nil
}

func (h *defaultRequestHandler) handleFSWrite(_ context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	if h.fs == nil {
		log.Warn("fs handler not configured")
		return nil, fmt.Errorf("filesystem access not available")
	}

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid fs/write_text_file params: %w", err)
	}

	absPath := h.resolvePath(req.Path)
	if absPath == "" {
		return nil, fmt.Errorf("invalid path: %s", req.Path)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(absPath)
	if err := h.fs.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	f, err := h.fs.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s for writing: %w", absPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write([]byte(req.Content)); err != nil {
		return nil, fmt.Errorf("failed to write file %s: %w", absPath, err)
	}

	log.Info("agent wrote file", "path", absPath, "bytes", len(req.Content))
	return map[string]interface{}{"status": "ok"}, nil
}

// resolvePath resolves a path against cwd and validates it stays within cwd.
// Both relative and absolute paths are confined to cwd to prevent directory traversal.
func (h *defaultRequestHandler) resolvePath(p string) string {
	if p == "" || h.cwd == "" {
		return ""
	}

	absPath := filepath.Clean(filepath.Join(h.cwd, p))

	// Path traversal check: ensure resolved path is within cwd
	if absPath != h.cwd && !strings.HasPrefix(absPath, h.cwd+string(filepath.Separator)) {
		return ""
	}

	return absPath
}

// --- Terminal methods ---

func (h *defaultRequestHandler) handleTerminalCreate(ctx context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	if h.proc == nil {
		log.Warn("terminal handler not configured")
		return nil, fmt.Errorf("terminal access not available")
	}

	var req struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Cwd     string   `json:"cwd"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid terminal/create params: %w", err)
	}
	if req.Command == "" {
		return nil, fmt.Errorf("terminal/create requires a 'command' field")
	}

	// Resolve cwd: use request cwd if provided and within sandbox, else handler cwd
	workDir := h.cwd
	if req.Cwd != "" {
		resolved := h.resolvePath(req.Cwd)
		if resolved != "" {
			workDir = resolved
		}
	}

	spec := middleware.CommandSpec{
		Runner: req.Command,
		Args:   req.Args,
		Dir:    workDir,
	}

	log.Info("agent executing command", "command", req.Command, "args", req.Args, "cwd", workDir)
	result, err := h.proc.ExecSeparate(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("command execution failed: %w", err)
	}

	// Truncate large outputs to prevent memory issues
	stdout := string(result.Stdout)
	stderr := string(result.Stderr)
	const maxOutputLen = 100 * 1024 // 100KB
	if len(stdout) > maxOutputLen {
		stdout = stdout[:maxOutputLen] + "\n... (truncated)"
	}
	if len(stderr) > maxOutputLen {
		stderr = stderr[:maxOutputLen] + "\n... (truncated)"
	}

	log.Info("command completed", "exit_code", result.ExitCode, "stdout_len", len(result.Stdout), "stderr_len", len(result.Stderr))
	return map[string]interface{}{
		"exitCode": result.ExitCode,
		"stdout":   stdout,
		"stderr":   stderr,
	}, nil
}

// --- Async terminal methods ---

// terminalIDFromParams extracts the terminalId from request params.
func terminalIDFromParams(params json.RawMessage) (string, error) {
	var req struct {
		TerminalID string `json:"terminalId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return "", fmt.Errorf("invalid params: %w", err)
	}
	if req.TerminalID == "" {
		return "", fmt.Errorf("missing terminalId")
	}
	return req.TerminalID, nil
}

// getTerminal retrieves a terminal session by ID.
func (h *defaultRequestHandler) getTerminal(id string) (*terminalSession, error) {
	h.terminalsMu.Lock()
	defer h.terminalsMu.Unlock()
	ts, ok := h.terminals[id]
	if !ok {
		return nil, fmt.Errorf("terminal %s not found", id)
	}
	return ts, nil
}

func (h *defaultRequestHandler) handleTerminalOutput(_ context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	id, err := terminalIDFromParams(params)
	if err != nil {
		return nil, err
	}
	ts, err := h.getTerminal(id)
	if err != nil {
		return nil, err
	}

	output := ts.output.String()
	log.Info("terminal output", "terminalId", id, "output_len", len(output))
	return map[string]interface{}{
		"output": output,
	}, nil
}

func (h *defaultRequestHandler) handleTerminalWaitForExit(ctx context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	id, err := terminalIDFromParams(params)
	if err != nil {
		return nil, err
	}
	ts, err := h.getTerminal(id)
	if err != nil {
		return nil, err
	}

	// Wait for the process to finish or context cancellation
	select {
	case <-ts.done:
		// Process already finished
	case <-ctx.Done():
		return nil, fmt.Errorf("wait cancelled: %w", ctx.Err())
	}

	output := ts.output.String()
	log.Info("terminal exited", "terminalId", id, "output_len", len(output))
	return map[string]interface{}{
		"exitCode": 0,
		"output":   output,
	}, nil
}

func (h *defaultRequestHandler) handleTerminalKill(_ context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	id, err := terminalIDFromParams(params)
	if err != nil {
		return nil, err
	}
	ts, err := h.getTerminal(id)
	if err != nil {
		return nil, err
	}

	if err := ts.id.Kill(); err != nil {
		log.Warn("failed to kill terminal", "terminalId", id, "error", err)
		return nil, fmt.Errorf("failed to kill terminal %s: %w", id, err)
	}
	ts.once.Do(func() { close(ts.done) })

	log.Info("terminal killed", "terminalId", id)
	return map[string]interface{}{"status": "ok"}, nil
}

func (h *defaultRequestHandler) handleTerminalRelease(_ context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	id, err := terminalIDFromParams(params)
	if err != nil {
		return nil, err
	}

	h.terminalsMu.Lock()
	ts, ok := h.terminals[id]
	if ok {
		delete(h.terminals, id)
	}
	h.terminalsMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("terminal %s not found", id)
	}

	// Ensure process is cleaned up
	_ = ts.id.Kill()
	ts.once.Do(func() { close(ts.done) })

	log.Info("terminal released", "terminalId", id)
	return map[string]interface{}{"status": "ok"}, nil
}

// CloseTerminals cleans up all active terminal sessions.
func (h *defaultRequestHandler) CloseTerminals() {
	h.terminalsMu.Lock()
	defer h.terminalsMu.Unlock()
	for id, ts := range h.terminals {
		_ = ts.id.Kill()
		ts.once.Do(func() { close(ts.done) })
		delete(h.terminals, id)
	}
}
