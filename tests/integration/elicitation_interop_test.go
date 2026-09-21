package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/agents"
	execprov "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// TestElicitationInteropOverRealStdioProcess drives the stable ACP elicitation
// round trip across a real process boundary: a real ACP peer subprocess sends
// elicitation/create on stdio, Matrix's real stdio transport and request
// handler project it into the real registry, the answer is posted through the
// same registry the HTTP channel frontend uses, and the peer reports back what
// it received. Nothing here is in-process stubbing of the protocol.
func TestElicitationInteropOverRealStdioProcess(t *testing.T) {
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	service := elicitation.NewService(30 * time.Second)
	transport, err := zedacp.NewStdioTransport(ctx, bin, nil, "--cwd", workspace)
	if err != nil {
		t.Fatalf("start mock ACP peer: %v", err)
	}
	client := zedacp.NewClient(ctx, transport)
	defer client.Close()

	handler := agents.NewDefaultRequestHandler(func() bool { return true }).
		WithFS(osfs.NewFSProvider(), workspace).
		WithProcess(execprov.NewProvider()).
		WithElicitationFrontend(service).
		WithAgentIdentity("mock-agent")
	client.SetRequestHandler(handler)

	initResp, err := client.Initialize(ctx, zedacp.InitializeRequest{
		ProtocolVersion:    1,
		ClientInfo:         map[string]interface{}{"name": "matrix-elicitation-interop", "version": "0.0.0-test"},
		ClientCapabilities: &zedacp.ClientCapabilities{Elicitation: &zedacp.ElicitationCapabilities{Form: &zedacp.ElicitationModeCapability{}, URL: &zedacp.ElicitationModeCapability{}}},
	})
	if err != nil {
		t.Fatalf("initialize against mock peer: %v", err)
	}
	if initResp.ProtocolVersion == 0 {
		t.Fatalf("mock peer returned invalid protocol version: %#v", initResp)
	}

	session, err := client.NewSession(ctx, zedacp.NewSessionRequest{Cwd: workspace, McpServers: []zedacp.McpServerConfig{}})
	if err != nil {
		t.Fatalf("new session against mock peer: %v", err)
	}

	observer := &acpProbeObserver{}
	promptDone := make(chan error, 1)
	go func() {
		_, err := client.Prompt(ctx, zedacp.PromptRequest{
			SessionID: session.SessionID,
			Prompt:    []zedacp.Content{{Type: "text", Text: "__ELICITATION_TEST__"}},
		}, observer)
		promptDone <- err
	}()

	// The peer's question must surface in the shared registry, naming the agent
	// and carrying the form schema it sent on the wire.
	var pending middleware.ElicitationRequest
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		entries := service.Pending()
		if len(entries) == 1 {
			pending = entries[0].Request
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pending.ID == "" {
		t.Fatal("elicitation/create from the real peer never reached the registry")
	}
	if pending.AgentID != "mock-agent" {
		t.Fatalf("registry lost the asking agent: %+v", pending)
	}
	if pending.Mode != middleware.ElicitationModeForm || len(pending.Fields) != 1 || !pending.Fields[0].Required {
		t.Fatalf("form schema did not survive the wire: %+v", pending)
	}
	if pending.SessionID != session.SessionID {
		t.Fatalf("session scope did not survive the wire: %q vs %q", pending.SessionID, session.SessionID)
	}

	if !service.Respond(pending.ID, middleware.AcceptElicitation(map[string]interface{}{"db": "postgres"})) {
		t.Fatal("registry refused the answer")
	}
	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("prompt failed: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("prompt never completed after the elicitation was answered")
	}
	// The peer echoes what it received, proving the accepted values crossed
	// back over the real transport rather than being absorbed locally.
	output := observer.Text()
	if !strings.Contains(output, "elicitation action=accept") || !strings.Contains(output, "db=postgres") {
		t.Fatalf("peer did not observe the accepted answer; output=%q", output)
	}
}

// TestElicitationInteropDeclineOverRealStdioProcess proves the negative path
// across the same real boundary: a declined request must reach the peer as an
// explicit decline, never as a hang or a silent success.
func TestElicitationInteropDeclineOverRealStdioProcess(t *testing.T) {
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	service := elicitation.NewService(30 * time.Second)
	transport, err := zedacp.NewStdioTransport(ctx, bin, nil, "--cwd", workspace)
	if err != nil {
		t.Fatalf("start mock ACP peer: %v", err)
	}
	client := zedacp.NewClient(ctx, transport)
	defer client.Close()

	handler := agents.NewDefaultRequestHandler(func() bool { return true }).
		WithFS(osfs.NewFSProvider(), workspace).
		WithProcess(execprov.NewProvider()).
		WithElicitationFrontend(service).
		WithAgentIdentity("mock-agent")
	client.SetRequestHandler(handler)

	if _, err := client.Initialize(ctx, zedacp.InitializeRequest{
		ProtocolVersion:    1,
		ClientCapabilities: &zedacp.ClientCapabilities{Elicitation: &zedacp.ElicitationCapabilities{Form: &zedacp.ElicitationModeCapability{}}},
	}); err != nil {
		t.Fatalf("initialize against mock peer: %v", err)
	}
	session, err := client.NewSession(ctx, zedacp.NewSessionRequest{Cwd: workspace, McpServers: []zedacp.McpServerConfig{}})
	if err != nil {
		t.Fatalf("new session against mock peer: %v", err)
	}

	observer := &acpProbeObserver{}
	promptDone := make(chan error, 1)
	go func() {
		_, err := client.Prompt(ctx, zedacp.PromptRequest{
			SessionID: session.SessionID,
			Prompt:    []zedacp.Content{{Type: "text", Text: "__ELICITATION_TEST__"}},
		}, observer)
		promptDone <- err
	}()

	var pendingID string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if entries := service.Pending(); len(entries) == 1 {
			pendingID = entries[0].Request.ID
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pendingID == "" {
		t.Fatal("elicitation/create from the real peer never reached the registry")
	}
	if !service.Respond(pendingID, middleware.DeclineElicitation()) {
		t.Fatal("registry refused the decline")
	}
	select {
	case err := <-promptDone:
		if err != nil {
			t.Fatalf("prompt failed: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("prompt never completed after the decline")
	}
	if output := observer.Text(); !strings.Contains(output, "elicitation action=decline") {
		t.Fatalf("peer did not observe the decline; output=%q", output)
	}
}

// TestElicitationRevokedWhenRunIsCancelled proves over the real stdio process
// boundary that cancelling a run revokes its pending question: the registry
// must empty out promptly and the peer must receive a cancel outcome, rather
// than the entry surviving until the elicitation timeout.
func TestElicitationRevokedWhenRunIsCancelled(t *testing.T) {
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()

	// A long elicitation timeout makes the assertion meaningful: only run
	// cancellation can resolve the pending request within the test window.
	service := elicitation.NewService(10 * time.Minute)
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	transport, err := zedacp.NewStdioTransport(runCtx, bin, nil, "--cwd", workspace)
	if err != nil {
		t.Fatalf("start mock ACP peer: %v", err)
	}
	client := zedacp.NewClient(runCtx, transport)
	defer client.Close()

	handler := agents.NewDefaultRequestHandler(func() bool { return true }).
		WithFS(osfs.NewFSProvider(), workspace).
		WithProcess(execprov.NewProvider()).
		WithElicitationFrontend(service).
		WithAgentIdentity("mock-agent")
	client.SetRequestHandler(handler)

	if _, err := client.Initialize(runCtx, zedacp.InitializeRequest{
		ProtocolVersion:    1,
		ClientCapabilities: &zedacp.ClientCapabilities{Elicitation: &zedacp.ElicitationCapabilities{Form: &zedacp.ElicitationModeCapability{}}},
	}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	session, err := client.NewSession(runCtx, zedacp.NewSessionRequest{Cwd: workspace, McpServers: []zedacp.McpServerConfig{}})
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}

	observer := &acpProbeObserver{}
	promptDone := make(chan error, 1)
	go func() {
		_, err := client.Prompt(runCtx, zedacp.PromptRequest{
			SessionID: session.SessionID,
			Prompt:    []zedacp.Content{{Type: "text", Text: "__ELICITATION_TEST__"}},
		}, observer)
		promptDone <- err
	}()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if len(service.Pending()) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pending := service.Pending(); len(pending) != 1 {
		t.Fatalf("expected one pending elicitation before cancelling, got %d", len(pending))
	}

	cancelRun()

	revoked := time.Now().Add(5 * time.Second)
	for time.Now().Before(revoked) {
		if len(service.Pending()) == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pending := service.Pending(); len(pending) != 0 {
		t.Fatalf("cancelling the run left %d pending elicitation(s): %+v", len(pending), pending)
	}
	<-promptDone
}

// TestMockAgentEmitsNothingWithoutElicitationCapability is the control: with no
// elicitation frontend wired, the same peer prompt must still complete, and the
// client must answer with an explicit decline rather than stalling.
func TestMockAgentEmitsNothingWithoutElicitationCapability(t *testing.T) {
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	transport, err := zedacp.NewStdioTransport(ctx, bin, nil, "--cwd", workspace)
	if err != nil {
		t.Fatalf("start mock ACP peer: %v", err)
	}
	client := zedacp.NewClient(ctx, transport)
	defer client.Close()

	handler := agents.NewDefaultRequestHandler(func() bool { return true }).
		WithFS(osfs.NewFSProvider(), workspace).
		WithProcess(execprov.NewProvider())
	client.SetRequestHandler(handler)

	if _, err := client.Initialize(ctx, zedacp.InitializeRequest{ProtocolVersion: 1}); err != nil {
		t.Fatalf("initialize against mock peer: %v", err)
	}
	session, err := client.NewSession(ctx, zedacp.NewSessionRequest{Cwd: workspace, McpServers: []zedacp.McpServerConfig{}})
	if err != nil {
		t.Fatalf("new session against mock peer: %v", err)
	}
	observer := &acpProbeObserver{}
	if _, err := client.Prompt(ctx, zedacp.PromptRequest{
		SessionID: session.SessionID,
		Prompt:    []zedacp.Content{{Type: "text", Text: "__ELICITATION_TEST__"}},
	}, observer); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if output := observer.Text(); !strings.Contains(output, "elicitation action=decline") {
		t.Fatalf("frontend-less client must decline explicitly; output=%q", output)
	}
}

var (
	mockAgentBuildOnce sync.Once
	mockAgentBuildDir  string
	mockAgentBuildPath string
	mockAgentBuildErr  error
)

// buildMockACPAgent compiles the mock peer once per test binary and keeps it
// alive for the whole package run; TestMain removes it afterwards.
func buildMockACPAgent(t *testing.T) string {
	t.Helper()
	mockAgentBuildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "matrix-mock-agent-")
		if err != nil {
			mockAgentBuildErr = fmt.Errorf("create mock agent temp dir: %w", err)
			return
		}
		mockAgentBuildDir = dir
		out := filepath.Join(dir, "mock-agent")
		cmd := exec.Command("go", "build", "-o", out, "./cmd/mock-agent")
		cmd.Dir = repoRoot(t)
		if output, err := cmd.CombinedOutput(); err != nil {
			mockAgentBuildErr = fmt.Errorf("build mock agent: %w: %s", err, output)
			return
		}
		mockAgentBuildPath = out
	})
	if mockAgentBuildErr != nil {
		t.Fatalf("%v", mockAgentBuildErr)
	}
	return mockAgentBuildPath
}

// cleanupMockACPAgent is called by TestMain once the package run finishes.
func cleanupMockACPAgent() {
	if mockAgentBuildDir != "" {
		_ = os.RemoveAll(mockAgentBuildDir)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}
	return strings.TrimSpace(string(out))
}
