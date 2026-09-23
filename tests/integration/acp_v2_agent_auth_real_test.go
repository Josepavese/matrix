package integration

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/agents"
	execprov "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// ----------------------------------------------------------------------------
// ACP v2 authentication against a real peer process
//
// The terminal authentication test next door proves the relaunch flow. These
// drive the other half of the version 2 authentication contract against the same
// real stdio peer:
//
//   - an agent-handled method is completed by auth/login on the running process,
//     the peer records the method name and the methodId it received, and the
//     gated prompt is retried on the same connection;
//   - auth/logout reaches the peer and puts the gate back, so the next gated
//     request has to authenticate again;
//   - a peer that offers only a terminal method while the operator has not opted
//     in is refused with the setting that would enable it, and nothing is run;
//   - the specification's error code alone (no marker in the data) is enough to
//     drive the login-and-retry, and
//   - a version 2 initialize does not advertise the client file system, terminal
//     execution or client session capabilities that generation removed.
//
// The peer is the repository's own binary, so these tests need no network and no
// installed agent.
// ----------------------------------------------------------------------------

// The names below are the contract with cmd/mock-agent/acp_v2.go, repeated here
// on purpose: a rename that only lands on one side has to fail the test.
const (
	mockAuthTypeEnv      = "MOCK_AGENT_V2_AUTH_TYPE"
	mockErrorShapeEnv    = "MOCK_AGENT_V2_ERROR_SHAPE"
	mockForceTerminalEnv = "MOCK_AGENT_V2_FORCE_TERMINAL"
	mockAuthTypeAgent    = "agent"
	mockAuthTypeTerminal = "terminal"
	mockErrorShapeCode   = "code"
	mockAgentMethodID    = "agent-login"
	mockAgentAuthAgentID = "mock-agent-v2-agent-auth"
)

// newV2AuthRouter builds a router that talks to the real v2 peer with the given
// advertised authentication type. Terminal authentication is left off unless the
// caller turns it on, so the opt-in tests are testing the real default.
func newV2AuthRouter(t *testing.T, bin, workspace, authType string, extraEnv ...string) (*agents.Router, string) {
	t.Helper()
	evidenceDir := t.TempDir()
	logPath := filepath.Join(evidenceDir, "peer-methods.jsonl")
	env := append([]string{
		mockLogEnv + "=" + logPath,
		mockCredentialEnv + "=" + filepath.Join(evidenceDir, "terminal-credential.json"),
		mockAuthTypeEnv + "=" + authType,
	}, extraEnv...)
	resolver := &acpV2MockResolver{bin: bin, args: []string{"--cwd", workspace, mockV2Flag}, env: env}
	router := agents.NewRouter(resolver)
	router.SetTrustMode(func() bool { return true })
	router.SetProcess(execprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), workspace)
	t.Cleanup(router.Close)
	return router, logPath
}

func routeV2Prompt(ctx context.Context, t *testing.T, router *agents.Router, workspace, logicalID string) (string, error) {
	t.Helper()
	output, _, _, _, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          mockV2AgentID,
		LogicalSessionID: logicalID,
		WorkspacePath:    workspace,
		Message:          "hello from the v2 authentication test",
	})
	return output, err
}

// TestACPv2AgentAuthenticationOverRealStdioProcess is the end-to-end proof of the
// agent-handled flow: the peer gates the prompt, Matrix calls auth/login with the
// advertised method id, and the retried prompt is answered on the same process.
func TestACPv2AgentAuthenticationOverRealStdioProcess(t *testing.T) {
	requireMockAgentBuild(t)
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()
	router, logPath := newV2AuthRouter(t, bin, workspace, mockAuthTypeAgent)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	output, err := routeV2Prompt(ctx, t, router, workspace, "acp-v2-agent-auth")
	if err != nil {
		t.Fatalf("the gated prompt never succeeded after an agent login: %v", err)
	}
	if !strings.Contains(output, mockAcceptedText) {
		t.Fatalf("the retried prompt must have been answered by the authenticated peer, output=%q", output)
	}

	records := readMockPeerLog(t, logPath)
	assertAgentLoginWentOverTheWire(t, records)
	assertNoTerminalLogin(t, records)
}

// TestACPv2LogoutPutsTheGateBackOverRealStdioProcess covers what happens after a
// logout: the specification says authentication-gated requests require a login
// again, so the next gated turn has to authenticate once more — which is also the
// proof that auth/logout reached a real peer under its version 2 name.
func TestACPv2LogoutPutsTheGateBackOverRealStdioProcess(t *testing.T) {
	requireMockAgentBuild(t)
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()
	router, logPath := newV2AuthRouter(t, bin, workspace, mockAuthTypeAgent)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if _, err := routeV2Prompt(ctx, t, router, workspace, "acp-v2-logout-before"); err != nil {
		t.Fatalf("the first gated turn must succeed after its login: %v", err)
	}
	if err := router.LogoutAgent(ctx, mockV2AgentID); err != nil {
		t.Fatalf("Matrix must be able to log out of a version 2 agent that advertises methods: %v", err)
	}
	output, err := routeV2Prompt(ctx, t, router, workspace, "acp-v2-logout-after")
	if err != nil {
		t.Fatalf("the gated turn after a logout must authenticate again: %v", err)
	}
	if !strings.Contains(output, mockAcceptedText) {
		t.Fatalf("the retried prompt after logout must be answered, output=%q", output)
	}

	records := readMockPeerLog(t, logPath)
	logouts := peerRecords(records, func(record mockPeerRecord) bool { return record.Method == "auth/logout" })
	if len(logouts) != 1 {
		t.Fatalf("auth/logout must reach the peer exactly once: %s", describePeerRecords(records))
	}
	logins := peerRecords(records, func(record mockPeerRecord) bool { return record.Method == "auth/login" })
	if len(logins) != 2 {
		t.Fatalf("the gate must be answered once per gated period, got %d logins: %s", len(logins), describePeerRecords(records))
	}
	prompts := peerRecords(records, func(record mockPeerRecord) bool { return record.Method == "session/prompt" })
	if len(prompts) != 4 {
		t.Fatalf("expected two gated prompts and two retries, got %d: %s", len(prompts), describePeerRecords(records))
	}
	for i, want := range []bool{false, true, false, true} {
		if prompts[i].Details["authenticated"] != want {
			t.Fatalf("prompt %d arrived authenticated=%v, want %v: %s", i, prompts[i].Details["authenticated"], want, describePeerRecord(prompts[i]))
		}
	}
	logoutIndex := indexOfPeerRecord(records, func(record mockPeerRecord) bool { return record.Method == "auth/logout" })
	secondPrompt := indicesOfPeerRecords(records, func(record mockPeerRecord) bool { return record.Method == "session/prompt" })[1]
	thirdPrompt := indicesOfPeerRecords(records, func(record mockPeerRecord) bool { return record.Method == "session/prompt" })[2]
	if logoutIndex < secondPrompt || logoutIndex > thirdPrompt {
		t.Fatalf("the logout must sit between the authenticated turn and the next gated one: %s", describePeerRecords(records))
	}
}

// TestACPv2TerminalOnlyMethodIsRefusedWithoutTheOperatorOptIn is the refusal the
// audit asks about: the peer offers a terminal method the operator has not opted
// into, so Matrix must report the setting that would enable it instead of
// silently pretending the agent published nothing — and it must not run anything.
func TestACPv2TerminalOnlyMethodIsRefusedWithoutTheOperatorOptIn(t *testing.T) {
	requireMockAgentBuild(t)
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()
	// The peer offers the terminal method even though Matrix did not advertise
	// capabilities.auth.terminal, which the specification forbids it to do. That
	// is exactly the peer the client has to refuse.
	router, logPath := newV2AuthRouter(t, bin, workspace, mockAuthTypeTerminal,
		mockForceTerminalEnv+"=true")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	_, err := routeV2Prompt(ctx, t, router, workspace, "acp-v2-terminal-refused")
	if err == nil {
		t.Fatal("a peer whose only method Matrix will not run must not be silently ignored")
	}
	if !strings.Contains(err.Error(), "agent.terminal_auth_enabled") {
		t.Fatalf("the refusal must name the operator opt-in that would run the method: %v", err)
	}
	records := readMockPeerLog(t, logPath)
	assertNoTerminalLogin(t, records)
	prompts := peerRecords(records, func(record mockPeerRecord) bool { return record.Method == "session/prompt" })
	if len(prompts) != 1 {
		t.Fatalf("a request with no runnable method must not be retried, got %d prompts: %s", len(prompts), describePeerRecords(records))
	}
	for _, record := range records {
		if record.Method == "auth/login" || record.Method == "authenticate" {
			t.Fatalf("no login may be attempted: %s", describePeerRecord(record))
		}
	}
}

// TestACPv2AuthenticationRequiredIsReadFromTheSpecificationCodeAlone proves the
// gate is understood from the error code the specification assigns it, without
// any marker in the data. A client that only reads the data would let this turn
// fail instead of logging in and retrying.
func TestACPv2AuthenticationRequiredIsReadFromTheSpecificationCodeAlone(t *testing.T) {
	requireMockAgentBuild(t)
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()
	router, logPath := newV2AuthRouter(t, bin, workspace, mockAuthTypeAgent,
		mockErrorShapeEnv+"="+mockErrorShapeCode)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	output, err := routeV2Prompt(ctx, t, router, workspace, "acp-v2-code-only-gate")
	if err != nil {
		t.Fatalf("the -32000 gate must be recognised without any data payload: %v", err)
	}
	if !strings.Contains(output, mockAcceptedText) {
		t.Fatalf("the retried prompt must be answered, output=%q", output)
	}
	assertAgentLoginWentOverTheWire(t, readMockPeerLog(t, logPath))
}

// TestACPv2InitializeDoesNotAdvertiseTheRemovedV1ClientCapabilities reads what
// the real peer received: version 2 deleted the client file system and terminal
// execution surfaces and defines no client session capability, so the handshake
// must advertise neither, while the capabilities both generations share stay.
func TestACPv2InitializeDoesNotAdvertiseTheRemovedV1ClientCapabilities(t *testing.T) {
	requireMockAgentBuild(t)
	bin := buildMockACPAgent(t)
	evidenceDir := t.TempDir()
	logPath := filepath.Join(evidenceDir, "peer-methods.jsonl")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := newRawMockV2Peer(ctx, t, bin, []string{
		mockLogEnv + "=" + logPath,
		mockCredentialEnv + "=" + filepath.Join(evidenceDir, "credential.json"),
	})
	defer client.Close()

	if _, err := client.Initialize(ctx, zedacp.InitializeRequest{
		ProtocolVersion: zedacp.ProtocolVersionV2,
		ClientInfo:      map[string]interface{}{"name": "matrix", "version": "1.0"},
		ClientCapabilities: &zedacp.ClientCapabilities{
			Fs:          &zedacp.FsCapability{ReadTextFile: true, WriteTextFile: true},
			Terminal:    true,
			Session:     &zedacp.ClientSessionCapabilities{ConfigOptions: &zedacp.SessionConfigOptionsCapabilities{Boolean: &zedacp.BooleanConfigOptionCapabilities{}}},
			Elicitation: &zedacp.ElicitationCapabilities{Form: &zedacp.ElicitationModeCapability{}},
			Auth:        &zedacp.AuthCapabilities{Terminal: &zedacp.TerminalAuthCapabilities{}},
		},
	}); err != nil {
		t.Fatalf("the v2 handshake must complete: %v", err)
	}

	records := readMockPeerLog(t, logPath)
	seen := peerRecords(records, func(record mockPeerRecord) bool { return record.Kind == "initialize_seen" })
	if len(seen) != 1 {
		t.Fatalf("expected one recorded handshake: %s", describePeerRecords(records))
	}
	if seen[0].Details["accepted"] != true {
		t.Fatalf("the handshake must have been accepted: %s", describePeerRecord(seen[0]))
	}
	keys, _ := seen[0].Details["capabilityKeys"].([]interface{})
	got := make([]string, 0, len(keys))
	for _, key := range keys {
		got = append(got, asString(key))
	}
	if strings.Join(got, ",") != "auth,elicitation" {
		t.Fatalf("a version 2 handshake may advertise only the shared capabilities, got %v: %s", got, describePeerRecord(seen[0]))
	}
	if seen[0].Details["authTerminalCapability"] != true {
		t.Fatalf("capabilities.auth.terminal must still be advertised: %s", describePeerRecord(seen[0]))
	}
}

// TestACPv2SessionSurfaceIsReadFromTheRealPeerOverStdio is the session half
// against a real peer: the peer advertises capabilities.session, so Matrix must
// report the baseline methods it implies, must not report session/load (removed
// in v2), and must name the authentication methods the generation defines.
func TestACPv2SessionSurfaceIsReadFromTheRealPeerOverStdio(t *testing.T) {
	requireMockAgentBuild(t)
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()
	router, _ := newV2AuthRouter(t, bin, workspace, mockAuthTypeAgent)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	report, err := router.AgentCapabilities(ctx, mockV2AgentID)
	if err != nil {
		t.Fatalf("Matrix must report the real v2 peer's capabilities: %v", err)
	}
	for _, operation := range []string{"session/list", "session/resume", "session/close", "auth/login"} {
		if !report.Operations[operation].Supported {
			t.Fatalf("capabilities.session and the advertised methods make %s supported: %#v", operation, report.Operations)
		}
	}
	if report.Operations["session/load"].Supported {
		t.Fatalf("version 2 removed session/load: %#v", report.Operations["session/load"])
	}
	if descriptor, ok := report.Operations["authenticate"]; ok && descriptor.Supported {
		t.Fatalf("version 2 renamed authenticate to auth/login: %#v", descriptor)
	}
	for _, operation := range []string{"fs/read_text_file", "terminal/create", "session/set_mode"} {
		if report.Operations[operation].Supported {
			t.Fatalf("%s is not part of a version 2 connection: %#v", operation, report.Operations[operation])
		}
	}
}

// assertAgentLoginWentOverTheWire pinpoints the agent-handled flow: one
// auth/login carrying the advertised methodId under the version 2 parameter name,
// and exactly one gated prompt followed by its retry on the same process.
func assertAgentLoginWentOverTheWire(t *testing.T, records []mockPeerRecord) {
	t.Helper()
	logins := peerRecords(records, func(record mockPeerRecord) bool { return record.Method == "auth/login" })
	if len(logins) != 1 {
		t.Fatalf("expected exactly one auth/login, got %s", describePeerRecords(records))
	}
	if got := asString(logins[0].Details["methodId"]); got != mockAgentMethodID {
		t.Fatalf("auth/login must carry the advertised methodId, got %q: %s", got, describePeerRecord(logins[0]))
	}
	prompts := peerRecords(records, func(record mockPeerRecord) bool { return record.Method == "session/prompt" })
	if len(prompts) != 2 {
		t.Fatalf("expected the gated prompt and exactly one retry, got %s", describePeerRecords(records))
	}
	if prompts[0].Details["authenticated"] != false || prompts[1].Details["authenticated"] != true {
		t.Fatalf("the first prompt must be gated and the retry authenticated: %s", describePeerRecords(records))
	}
	if prompts[0].PID != prompts[1].PID {
		t.Fatalf("an agent-handled login keeps the connection; the retry ran on pid %d instead of %d",
			prompts[1].PID, prompts[0].PID)
	}
	for _, record := range records {
		if record.Method == "authenticate" {
			t.Fatalf("version 2 renamed authenticate to auth/login: %s", describePeerRecord(record))
		}
	}
}

// assertNoTerminalLogin keeps the agent-handled flows from silently running the
// terminal program, and the refusal path from launching anything at all.
func assertNoTerminalLogin(t *testing.T, records []mockPeerRecord) {
	t.Helper()
	if logins := peerRecords(records, func(record mockPeerRecord) bool { return record.Kind == "terminal_login" }); len(logins) != 0 {
		t.Fatalf("no terminal login process may run here: %s", describePeerRecords(logins))
	}
}
