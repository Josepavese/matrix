package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/providers/agents"
	execprov "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

type realACPProviderSpec struct {
	name string
	bin  string
	args []string
	env  []string
}

type acpProbeObserver struct {
	mu      sync.Mutex
	text    strings.Builder
	updates []string
}

func (o *acpProbeObserver) OnUpdate(notification zedacp.SessionNotification) {
	o.mu.Lock()
	defer o.mu.Unlock()
	update := notification.Update
	if update.SessionUpdate != "" {
		o.updates = append(o.updates, update.SessionUpdate)
	}
	for _, content := range update.Contents {
		if content.Text != "" {
			o.text.WriteString(content.Text)
		}
	}
	if len(update.Contents) == 0 && update.Content.Text != "" {
		o.text.WriteString(update.Content.Text)
	}
}

func (o *acpProbeObserver) Text() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.text.String()
}

func (o *acpProbeObserver) UpdateKinds() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]string, len(o.updates))
	copy(out, o.updates)
	return out
}

type countingACPHandler struct {
	inner zedacp.RequestHandler
	mu    sync.Mutex
	calls map[string]int
}

func newCountingACPHandler(inner zedacp.RequestHandler) *countingACPHandler {
	return &countingACPHandler{inner: inner, calls: map[string]int{}}
}

func (h *countingACPHandler) HandleRequest(ctx context.Context, method string, params json.RawMessage) (interface{}, error) {
	h.mu.Lock()
	h.calls[method]++
	h.mu.Unlock()
	return h.inner.HandleRequest(ctx, method, params)
}

func (h *countingACPHandler) Calls() map[string]int {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string]int, len(h.calls))
	for method, count := range h.calls {
		out[method] = count
	}
	return out
}

func TestSmoke_RealACPProviderLifecycleCompliance(t *testing.T) {
	requireSmokeTest(t)
	specs := realACPProviderSpecs(t)
	minProviders := realACPMinProviders()
	if len(specs) < minProviders {
		t.Fatalf("real ACP smoke requires at least %d providers, got %d: %#v", minProviders, len(specs), specs)
	}
	for _, spec := range specs {
		t.Run(spec.name, func(t *testing.T) {
			probeRealACPProvider(t, spec)
		})
	}
}

func realACPMinProviders() int {
	raw := strings.TrimSpace(os.Getenv("MATRIX_REAL_ACP_MIN_PROVIDERS"))
	if raw == "1" {
		return 1
	}
	if raw == "2" {
		return 2
	}
	return 3
}

func realACPProviderSpecs(t *testing.T) []realACPProviderSpec {
	if raw := strings.TrimSpace(os.Getenv("MATRIX_REAL_ACP_PROVIDERS")); raw != "" {
		return parseRealACPProviderSpecs(t, raw)
	}
	candidates := []realACPProviderSpec{
		{name: "opencode", bin: "opencode", args: []string{"acp", "--pure"}},
		{name: "codex", bin: "codex-acp"},
		{name: "gemini", bin: "gemini", args: []string{"--acp", "--yolo"}},
	}
	specs := make([]realACPProviderSpec, 0, len(candidates))
	for _, candidate := range candidates {
		bin, err := exec.LookPath(candidate.bin)
		if err != nil {
			t.Logf("provider %s unavailable: %v", candidate.name, err)
			continue
		}
		candidate.bin = bin
		specs = append(specs, candidate)
	}
	return specs
}

func parseRealACPProviderSpecs(t *testing.T, raw string) []realACPProviderSpec {
	t.Helper()
	var specs []realACPProviderSpec
	for _, item := range strings.Split(raw, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		name, command, ok := strings.Cut(item, "=")
		if !ok {
			t.Fatalf("invalid MATRIX_REAL_ACP_PROVIDERS item %q, expected name=command args", item)
		}
		fields := strings.Fields(command)
		if len(fields) == 0 {
			t.Fatalf("invalid empty command for provider %q", name)
		}
		bin, err := exec.LookPath(fields[0])
		if err != nil {
			t.Fatalf("provider %s binary %q not found: %v", name, fields[0], err)
		}
		specs = append(specs, realACPProviderSpec{name: strings.TrimSpace(name), bin: bin, args: fields[1:]})
	}
	return specs
}

func probeRealACPProvider(t *testing.T, spec realACPProviderSpec) {
	t.Helper()
	workspace := t.TempDir()
	fileToken := "MATRIX_FILE_" + strings.ToUpper(spec.name)
	terminalToken := "MATRIX_TERMINAL_" + strings.ToUpper(spec.name)
	replyToken := "MATRIX_ACP_REPLY_" + strings.ToUpper(spec.name)
	probeFile := filepath.Join(workspace, "acp_probe.txt")
	if err := os.WriteFile(probeFile, []byte(fileToken+"\n"), 0o644); err != nil {
		t.Fatalf("write probe file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	args := append([]string{}, spec.args...)
	if spec.name == "opencode" {
		args = append(args, "--cwd", workspace)
	}
	transport, err := zedacp.NewStdioTransport(ctx, spec.bin, spec.env, args...)
	if err != nil {
		t.Fatalf("start provider %s: %v", spec.name, err)
	}
	client := zedacp.NewClient(ctx, transport)
	defer client.Close()

	handler := newCountingACPHandler(
		agents.NewDefaultRequestHandler(func() bool { return true }).
			WithFS(osfs.NewFSProvider(), workspace).
			WithProcess(execprov.NewProvider()),
	)
	client.SetRequestHandler(handler)

	initResp, err := client.Initialize(ctx, zedacp.InitializeRequest{
		ProtocolVersion: 1,
		ClientInfo: map[string]interface{}{
			"name":    "matrix-real-acp-probe",
			"version": "0.0.0-test",
		},
		ClientCapabilities: &zedacp.ClientCapabilities{
			Fs:       &zedacp.FsCapability{ReadTextFile: true, WriteTextFile: true},
			Terminal: true,
		},
	})
	if err != nil {
		t.Fatalf("initialize provider %s: %v", spec.name, err)
	}
	if initResp.ProtocolVersion == 0 {
		t.Fatalf("provider %s returned invalid protocol version: %#v", spec.name, initResp)
	}
	t.Logf("provider=%s version=%d capabilities=%#v", spec.name, initResp.ProtocolVersion, initResp.Capabilities)

	session, err := client.NewSession(ctx, zedacp.NewSessionRequest{
		Cwd:        workspace,
		McpServers: []zedacp.McpServerConfig{},
	})
	if err != nil {
		t.Fatalf("new session provider %s: %v", spec.name, err)
	}
	if session.SessionID == "" {
		t.Fatalf("provider %s returned empty session id", spec.name)
	}

	probeConfigOptions(ctx, t, client, spec.name, session.SessionID, session.ConfigOptions)
	probeSessionDiscovery(ctx, t, client, initResp.Capabilities, workspace, session.SessionID)

	observer := &acpProbeObserver{}
	prompt := fmt.Sprintf(
		"ACP compliance probe. Reply with %s. Also read %s and include %s. If terminal execution is available, run `printf %s` and include that output. Keep the final answer under 30 words.",
		replyToken,
		probeFile,
		fileToken,
		terminalToken,
	)
	promptResp, err := client.Prompt(ctx, zedacp.PromptRequest{
		SessionID: session.SessionID,
		Prompt:    []zedacp.Content{{Type: "text", Text: prompt}},
	}, observer)
	if err != nil {
		t.Fatalf("prompt provider %s: %v", spec.name, err)
	}
	output := observer.Text()
	t.Logf("provider=%s stop=%s updates=%v handler_calls=%v output=%q", spec.name, promptResp.StopReason, observer.UpdateKinds(), handler.Calls(), output)
	if !strings.Contains(output, replyToken) {
		t.Fatalf("provider %s did not prove LLM prompt processing with token %s; output=%q", spec.name, replyToken, output)
	}
	if !strings.Contains(output, fileToken) {
		t.Fatalf("provider %s did not include file probe token %s; output=%q", spec.name, fileToken, output)
	}

	if supportsSessionCapability(initResp.Capabilities, "close") {
		if err := client.CloseSession(ctx, session.SessionID); err != nil {
			t.Fatalf("close session provider %s: %v", spec.name, err)
		}
		return
	}
	if err := client.CancelSession(ctx, session.SessionID); err != nil {
		t.Fatalf("cancel session provider %s: %v", spec.name, err)
	}
}

func probeConfigOptions(ctx context.Context, t *testing.T, client zedacp.ClientAPI, provider, sessionID string, options []zedacp.ConfigOption) {
	t.Helper()
	configID, value, ok := firstSafeConfigValue(options)
	if !ok {
		t.Logf("provider=%s no session config options advertised", provider)
		return
	}
	resp, err := client.SetConfigOption(ctx, zedacp.SetSessionConfigOptionRequest{
		SessionID: sessionID,
		ConfigID:  configID,
		Value:     value,
	})
	if err != nil {
		t.Fatalf("set config option provider %s: %v", provider, err)
	}
	if len(resp.ConfigOptions) == 0 {
		t.Fatalf("provider %s set_config_option returned no configOptions", provider)
	}
}

func probeSessionDiscovery(ctx context.Context, t *testing.T, client zedacp.ClientAPI, capabilities map[string]interface{}, cwd, sessionID string) {
	t.Helper()
	if supportsSessionCapability(capabilities, "list") {
		sessions, err := client.ListSessionsWithRequest(ctx, zedacp.ListSessionsRequest{Cwd: cwd})
		if err != nil {
			t.Logf("session/list advertised but unavailable for this probe session: %v", err)
		} else {
			t.Logf("session/list returned %d sessions", len(sessions.Sessions))
		}
	}
	if supportsSessionCapability(capabilities, "resume") {
		if _, err := client.ResumeSession(ctx, zedacp.ResumeSessionRequest{
			SessionID:  sessionID,
			Cwd:        cwd,
			McpServers: []zedacp.McpServerConfig{},
		}); err != nil {
			t.Logf("session/resume advertised but unavailable for this probe session: %v", err)
		} else {
			t.Logf("session/resume succeeded")
		}
		return
	}
	if supportsLoadSessionCapability(capabilities) {
		if _, err := client.LoadSession(ctx, zedacp.LoadSessionRequest{
			SessionID:  sessionID,
			Cwd:        cwd,
			McpServers: []zedacp.McpServerConfig{},
		}, nil); err != nil {
			t.Logf("session/load advertised but unavailable for this probe session: %v", err)
		} else {
			t.Logf("session/load succeeded")
		}
	}
}

func firstSafeConfigValue(options []zedacp.ConfigOption) (string, string, bool) {
	for _, option := range options {
		if option.ID == "" {
			continue
		}
		if option.Current != "" {
			return option.ID, option.Current, true
		}
		if len(option.Options) > 0 && option.Options[0].ID != "" {
			return option.ID, option.Options[0].ID, true
		}
	}
	return "", "", false
}

func supportsLoadSessionCapability(capabilities map[string]interface{}) bool {
	value, ok := capabilities["loadSession"]
	return ok && truthyACPCapability(value)
}

func supportsSessionCapability(capabilities map[string]interface{}, name string) bool {
	raw, ok := capabilities["sessionCapabilities"]
	if !ok {
		return false
	}
	sessionCaps, ok := raw.(map[string]interface{})
	if !ok {
		return false
	}
	value, ok := sessionCaps[name]
	return ok && truthyACPCapability(value)
}

func truthyACPCapability(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case nil:
		return false
	case map[string]interface{}:
		return true
	default:
		return true
	}
}
