package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	goexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Josepavese/matrix/internal/logic/frontendevents"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
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
	trustMode  func() bool        // returns true if auto-approve is enabled; defaults to true if nil
	fs         middleware.FS      // filesystem operations for fs/* methods
	proc       middleware.Process // process execution for terminal/* methods
	cwd        string             // working directory for path validation
	notifier   middleware.ThoughtNotifier
	notifierMu sync.Mutex
	extension  ExtensionRequestHandler

	// terminalRegistry holds active terminal sessions for async terminal methods.
	terminals      map[string]*terminalSession
	terminalsMu    sync.Mutex
	nextTerminalID uint64
}

// terminalSession tracks a running terminal process and its accumulated output.
type terminalSession struct {
	id              middleware.PipedProcess
	output          bytes.Buffer
	outputByteLimit int
	truncated       bool
	exitCode        *int
	signal          string
	done            chan struct{}
	once            sync.Once
	mu              sync.Mutex
	toolSignal      frontendevents.ToolSignal
	handler         *defaultRequestHandler
}

// ExtensionRequestHandler handles negotiated ACP extension methods. Matrix does
// not auto-approve unknown methods because that hides protocol drift.
type ExtensionRequestHandler func(ctx context.Context, method string, params json.RawMessage) (interface{}, error)

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

func (h *defaultRequestHandler) WithExtensionHandler(handler ExtensionRequestHandler) *defaultRequestHandler {
	h.extension = handler
	return h
}

func (h *defaultRequestHandler) WithNotifier(notifier middleware.ThoughtNotifier) *defaultRequestHandler {
	h.notifierMu.Lock()
	defer h.notifierMu.Unlock()
	h.notifier = notifier
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
		if h.extension != nil {
			return h.extension(ctx, method, params)
		}
		log.Warn("unsupported agent request", "event", "request_unsupported", "method", method)
		return nil, zedacp.NewMethodNotFoundError(method)
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
			h.notifyPermission(permissionAudit{params: params, decision: "approved", optionID: "allow-once", auto: true})
			return h.approveResponse("allow-once"), nil
		}
		h.notifyPermission(permissionAudit{params: params, decision: "denied"})
		return h.denyResponse(), nil
	}

	if !h.isTrustMode() {
		log.Info("denying permission (trust mode off)", "event", "permission_denied", "options_count", len(req.Options))
		h.notifyPermission(permissionAudit{params: params, options: req.Options, decision: "denied"})
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
	h.notifyPermission(permissionAudit{params: params, options: req.Options, decision: "approved", optionID: optionID, auto: true})
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
		Path  string `json:"path"`
		Line  *int   `json:"line,omitempty"`
		Limit *int   `json:"limit,omitempty"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid fs/read_text_file params: %w", err)
	}

	absPath := h.resolvePath(req.Path)
	if absPath == "" {
		return nil, fmt.Errorf("invalid path: %s", req.Path)
	}

	signal := h.beginClientTool(frontendevents.ACPClientFSToolSignal("fs/read_text_file", absPath, -1), params)
	data, err := h.fs.ReadFile(absPath)
	if err != nil {
		h.failClientTool(signal)
		return nil, fmt.Errorf("failed to read file %s: %w", absPath, err)
	}
	content := sliceTextLines(string(data), req.Line, req.Limit)
	signal.Metadata["bytes"] = len(content)
	h.completeClientTool(signal)

	log.Info("agent read file", "path", absPath, "bytes", len(content))
	return map[string]interface{}{
		"content": content,
	}, nil
}

func sliceTextLines(content string, line, limit *int) string {
	if line == nil && limit == nil {
		return content
	}
	lines := strings.SplitAfter(content, "\n")
	if len(lines) == 1 && lines[0] == "" {
		return ""
	}
	start := 0
	if line != nil && *line > 1 {
		start = *line - 1
	}
	if start >= len(lines) {
		return ""
	}
	end := len(lines)
	if limit != nil && *limit >= 0 && start+*limit < end {
		end = start + *limit
	}
	return strings.Join(lines[start:end], "")
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

	signal := h.beginClientTool(frontendevents.ACPClientFSToolSignal("fs/write_text_file", absPath, len(req.Content)), params)

	// Ensure parent directory exists
	dir := filepath.Dir(absPath)
	if err := h.fs.MkdirAll(dir, 0755); err != nil {
		h.failClientTool(signal)
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	f, err := h.fs.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		h.failClientTool(signal)
		return nil, fmt.Errorf("failed to open file %s for writing: %w", absPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write([]byte(req.Content)); err != nil {
		h.failClientTool(signal)
		return nil, fmt.Errorf("failed to write file %s: %w", absPath, err)
	}
	h.completeClientTool(signal)

	log.Info("agent wrote file", "path", absPath, "bytes", len(req.Content))
	return map[string]interface{}{"status": "ok"}, nil
}

// resolvePath resolves a path and validates it stays within cwd. ACP asks for
// absolute paths, but relative paths are still accepted for tolerant adapters.
func (h *defaultRequestHandler) resolvePath(p string) string {
	if p == "" || h.cwd == "" {
		return ""
	}

	cwd := filepath.Clean(h.cwd)
	absPath := filepath.Clean(p)
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Clean(filepath.Join(cwd, p))
	}

	// Path traversal check: ensure resolved path is within cwd
	if absPath != cwd && !strings.HasPrefix(absPath, cwd+string(filepath.Separator)) {
		return ""
	}

	return absPath
}

// --- Terminal methods ---

func (h *defaultRequestHandler) handleTerminalCreate(_ context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	if h.proc == nil {
		log.Warn("terminal handler not configured")
		return nil, fmt.Errorf("terminal access not available")
	}

	req, err := h.parseTerminalCreateRequest(params)
	if err != nil {
		return nil, err
	}

	log.Info("agent executing command", "command", req.Command, "args", req.Args, "cwd", req.WorkDir)
	signal := h.beginClientTool(frontendevents.ACPClientTerminalToolSignal(req.Command, req.Args, req.WorkDir, nil), params)
	pp, err := h.proc.StartPiped(req.commandSpec())
	if err != nil {
		h.failClientTool(signal)
		return nil, fmt.Errorf("command start failed: %w", err)
	}

	terminalID := h.nextACPClientTerminalID()
	ts := &terminalSession{
		id:              pp,
		done:            make(chan struct{}),
		outputByteLimit: terminalOutputLimit(req.OutputByteLimit),
		toolSignal:      signal,
		handler:         h,
	}
	h.terminalsMu.Lock()
	h.terminals[terminalID] = ts
	h.terminalsMu.Unlock()

	go ts.capture(terminalID, log)
	return map[string]interface{}{
		"terminalId": terminalID,
	}, nil
}

const defaultTerminalOutputByteLimit = 1024 * 1024

func (h *defaultRequestHandler) nextACPClientTerminalID() string {
	next := atomic.AddUint64(&h.nextTerminalID, 1)
	return fmt.Sprintf("terminal-%d", next)
}

func terminalOutputLimit(requested int) int {
	if requested <= 0 {
		return defaultTerminalOutputByteLimit
	}
	return requested
}

func (ts *terminalSession) capture(terminalID string, log *slog.Logger) {
	readErr := ts.copyOutput(ts.id.Stdout())
	waitErr := ts.id.Wait()
	code, signal := terminalExitStatus(waitErr)

	ts.mu.Lock()
	ts.exitCode = code
	ts.signal = signal
	ts.mu.Unlock()

	resultSignal := ts.toolSignal
	resultSignal.Metadata = cloneMetadata(resultSignal.Metadata)
	if code != nil {
		resultSignal.Metadata["exit_code"] = *code
	}
	if signal != "" {
		resultSignal.Metadata["signal"] = signal
	}
	if readErr != nil {
		resultSignal.Metadata["read_error"] = readErr.Error()
	}
	if waitErr == nil || (code != nil && *code == 0) {
		ts.handler.completeClientTool(resultSignal)
	} else {
		ts.handler.failClientTool(resultSignal)
	}
	log.Info("terminal completed", "terminalId", terminalID, "exit_code", codeValue(code), "signal", signal, "read_error", readErr, "wait_error", waitErr)
	ts.once.Do(func() { close(ts.done) })
}

func (ts *terminalSession) copyOutput(r io.Reader) error {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			ts.appendOutput(buf[:n])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func (ts *terminalSession) appendOutput(chunk []byte) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.outputByteLimit <= 0 {
		ts.output.Write(chunk)
		return
	}
	remaining := ts.outputByteLimit - ts.output.Len()
	if remaining <= 0 {
		ts.truncated = true
		return
	}
	if len(chunk) > remaining {
		ts.output.Write(chunk[:remaining])
		ts.truncated = true
		return
	}
	ts.output.Write(chunk)
}

func terminalExitStatus(err error) (*int, string) {
	if err == nil {
		code := 0
		return &code, ""
	}
	var exitErr *goexec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		if code >= 0 {
			return &code, ""
		}
		return nil, "terminated"
	}
	return nil, "error"
}

func codeValue(code *int) interface{} {
	if code == nil {
		return nil
	}
	return *code
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

	output, truncated, exitCode, signal, done := ts.snapshot()
	log.Info("terminal output", "terminalId", id, "output_len", len(output))
	resp := map[string]interface{}{
		"output":    output,
		"truncated": truncated,
	}
	if done {
		resp["exitStatus"] = map[string]interface{}{
			"exitCode": codeValue(exitCode),
			"signal":   nullableString(signal),
		}
	}
	return resp, nil
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

	_, _, exitCode, signal, _ := ts.snapshot()
	log.Info("terminal exited", "terminalId", id, "exit_code", codeValue(exitCode), "signal", signal)
	return map[string]interface{}{
		"exitCode": codeValue(exitCode),
		"signal":   nullableString(signal),
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

	log.Info("terminal killed", "terminalId", id)
	return map[string]interface{}{}, nil
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

	log.Info("terminal released", "terminalId", id)
	return map[string]interface{}{}, nil
}

// CloseTerminals cleans up all active terminal sessions.
func (h *defaultRequestHandler) CloseTerminals() {
	h.terminalsMu.Lock()
	defer h.terminalsMu.Unlock()
	for id, ts := range h.terminals {
		_ = ts.id.Kill()
		delete(h.terminals, id)
	}
}

func (ts *terminalSession) snapshot() (string, bool, *int, string, bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	done := false
	select {
	case <-ts.done:
		done = true
	default:
	}
	var code *int
	if ts.exitCode != nil {
		value := *ts.exitCode
		code = &value
	}
	return ts.output.String(), ts.truncated, code, ts.signal, done
}

func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}
