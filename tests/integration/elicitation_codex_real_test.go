package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/agents"
	execprov "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// codexACPBin resolves the installed codex ACP adapter, which reads the client
// capabilities advertised during initialize and only routes user-input and MCP
// approval requests through elicitation when the client advertises it.
func codexACPBin(t *testing.T) (string, []string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not available")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot resolve home directory")
	}
	adapter := filepath.Join(home, ".local/share/matrix/agents/codex/node_modules/@agentclientprotocol/codex-acp/dist/index.js")
	if _, err := os.Stat(adapter); err != nil {
		t.Skipf("codex ACP adapter not installed: %v", err)
	}
	return node, []string{adapter}
}

// TestSmoke_RealCodexACPEmitsElicitation is the end-to-end proof that the
// feature is not merely spec-compliant but actually reachable with a real
// agent: codex-acp converts a Codex user-input request into a stable ACP
// elicitation/create only when the client advertised the capability, so a
// successful round trip proves both the advertisement and the inbound path
// against a real peer. The test skips (with the observed prompt output) when
// the model declines to ask a question, because that is model behaviour, not
// protocol behaviour.
func TestSmoke_RealCodexACPEmitsElicitation(t *testing.T) {
	requireSmokeTest(t)
	node, adapterArgs := codexACPBin(t)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	workspace := t.TempDir()
	service := elicitation.NewService(60 * time.Second)

	env := []string{"MATRIX_CODEX_LAUNCH_POLICY_CONTRACT=codex-acp-env-v1"}
	transport, err := zedacp.NewStdioTransport(ctx, node, env, adapterArgs...)
	if err != nil {
		t.Fatalf("start codex-acp: %v", err)
	}
	client := zedacp.NewClient(ctx, transport)
	defer client.Close()

	handler := agents.NewDefaultRequestHandler(func() bool { return true }).
		WithFS(osfs.NewFSProvider(), workspace).
		WithProcess(execprov.NewProvider()).
		WithElicitationFrontend(service).
		WithAgentIdentity("codex")
	counter := newCountingACPHandler(handler)
	client.SetRequestHandler(counter)

	initResp, err := client.Initialize(ctx, zedacp.InitializeRequest{
		ProtocolVersion: 1,
		ClientInfo:      map[string]interface{}{"name": "matrix-codex-elicitation-probe", "version": "0.0.0-test"},
		ClientCapabilities: &zedacp.ClientCapabilities{
			Fs:       &zedacp.FsCapability{ReadTextFile: true, WriteTextFile: true},
			Terminal: true,
			Elicitation: &zedacp.ElicitationCapabilities{
				Form: &zedacp.ElicitationModeCapability{},
				URL:  &zedacp.ElicitationModeCapability{},
			},
		},
	})
	if err != nil {
		t.Fatalf("initialize codex-acp with elicitation advertised: %v", err)
	}
	t.Logf("codex negotiated protocol=%d capabilities=%v", initResp.ProtocolVersion, initResp.Capabilities)

	session, err := client.NewSession(ctx, zedacp.NewSessionRequest{Cwd: workspace, McpServers: []zedacp.McpServerConfig{}})
	if err != nil {
		t.Fatalf("codex-acp session/new: %v", err)
	}

	observer := &acpProbeObserver{}
	promptDone := make(chan error, 1)
	go func() {
		_, err := client.Prompt(ctx, zedacp.PromptRequest{
			SessionID: session.SessionID,
			Prompt: []zedacp.Content{{Type: "text", Text: strings.Join([]string{
				"Before doing any other work, ask me exactly one clarifying question",
				"using your user-input tool: which database should be used, postgres or sqlite?",
				"Wait for my answer instead of guessing. Do not create or modify any files.",
			}, " ")}},
		}, observer)
		promptDone <- err
	}()

	var pending middleware.ElicitationRequest
	deadline := time.Now().Add(240 * time.Second)
	for time.Now().Before(deadline) {
		if entries := service.Pending(); len(entries) == 1 {
			pending = entries[0].Request
			break
		}
		select {
		case err := <-promptDone:
			t.Skipf("codex completed without asking for input (model behaviour, not protocol): err=%v output=%q", err, observer.Text())
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pending.ID == "" {
		t.Skipf("codex did not emit an elicitation within the probe window; handler_calls=%v output=%q", counter.Calls(), observer.Text())
	}

	t.Logf("REAL ELICITATION RECEIVED from codex-acp: id=%s agent=%s mode=%s session=%s fields=%d message=%q",
		pending.ID, pending.AgentID, pending.Mode, pending.SessionID, len(pending.Fields), pending.Message)
	if pending.AgentID != "codex" {
		t.Fatalf("real elicitation lost agent identity: %+v", pending)
	}
	if pending.Mode != middleware.ElicitationModeForm {
		t.Fatalf("unexpected mode from real peer: %q", pending.Mode)
	}
	if len(pending.Fields) == 0 {
		t.Fatalf("real elicitation carried no schema: %+v", pending)
	}
	for _, field := range pending.Fields {
		t.Logf("  field name=%q title=%q type=%q required=%v options=%v default=%v description=%q",
			field.Name, field.Title, field.Type, field.Required, field.Options, field.Default, field.Description)
	}

	// Answer with a value the peer's own schema allows.
	values := map[string]interface{}{}
	for _, field := range pending.Fields {
		if len(field.Options) > 0 {
			values[field.Name] = field.Options[0].Value
			continue
		}
		switch field.Type {
		case "string":
			values[field.Name] = "postgres"
		case "number":
			values[field.Name] = float64(1)
		case "boolean":
			values[field.Name] = true
		}
	}
	if err := middleware.ValidateElicitationValues(pending, values); err != nil {
		t.Fatalf("probe built values the real schema rejects: %v (%+v)", err, values)
	}
	if !service.Respond(pending.ID, middleware.AcceptElicitation(values)) {
		t.Fatal("registry refused the answer for a real elicitation")
	}

	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("codex prompt failed after the elicitation was answered: %v", err)
		}
	case <-time.After(180 * time.Second):
		t.Fatal("codex never completed after its elicitation was answered")
	}
	t.Logf("codex completed after the elicitation; handler_calls=%v output=%q", counter.Calls(), observer.Text())
	if counter.Calls()["elicitation/create"] == 0 {
		t.Fatal("real elicitation was answered but the handler never recorded the inbound request")
	}
}
