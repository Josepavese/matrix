package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// ----------------------------------------------------------------------------
// Doubles for the ACP v2 authentication surface
// ----------------------------------------------------------------------------

// recordingProcess is the process double for terminal authentication: it records
// the launch spec Matrix derived from the endpoint and the method, and answers
// with a scripted exit status instead of running anything.
type recordingProcess struct {
	specs    []middleware.CommandSpec
	exitCode int
	runErr   error
	onRun    func()
}

func (p *recordingProcess) ExecSeparate(_ context.Context, spec middleware.CommandSpec) (*middleware.ExecResult, error) {
	p.specs = append(p.specs, spec)
	if p.onRun != nil {
		p.onRun()
	}
	if p.runErr != nil {
		return nil, p.runErr
	}
	return &middleware.ExecResult{ExitCode: p.exitCode}, nil
}

func (p *recordingProcess) Exec(middleware.CommandSpec) ([]byte, error) { return nil, nil }
func (p *recordingProcess) Start(middleware.CommandSpec) (middleware.ProcessHandle, error) {
	return nil, nil
}
func (p *recordingProcess) StartPiped(middleware.CommandSpec) (middleware.PipedProcess, error) {
	return nil, nil
}
func (p *recordingProcess) RunPrivileged(middleware.CommandSpec) ([]byte, error) { return nil, nil }
func (p *recordingProcess) HasExecutable(string) bool                            { return false }
func (p *recordingProcess) SpawnPTY() error                                      { return nil }

// scriptedAuthACPClient drives the authentication paths deterministically: it
// fails a scripted number of prompts with the structured marker and records what
// it was asked to authenticate with.
type scriptedAuthACPClient struct {
	*pagedListACPClient

	mu        sync.Mutex
	prompts   int
	failCount int
	promptErr error
	authCalls []string
	onPrompt  func()
}

func newScriptedAuthACPClient(version int) *scriptedAuthACPClient {
	return &scriptedAuthACPClient{
		pagedListACPClient: &pagedListACPClient{ctx: context.Background(), protocolVersion: version},
	}
}

func (c *scriptedAuthACPClient) Authenticate(ctx context.Context, methodID string) error {
	c.mu.Lock()
	c.authCalls = append(c.authCalls, methodID)
	c.mu.Unlock()
	return c.pagedListACPClient.Authenticate(ctx, methodID)
}

func (c *scriptedAuthACPClient) Prompt(ctx context.Context, req acpPromptRequest, observer acpSessionObserver) (*acpPromptResponse, error) {
	c.mu.Lock()
	c.prompts++
	call := c.prompts
	hook := c.onPrompt
	c.mu.Unlock()
	if hook != nil {
		hook()
	}
	if call <= c.failCount {
		return nil, c.promptErr
	}
	return c.pagedListACPClient.Prompt(ctx, req, observer)
}

func (c *scriptedAuthACPClient) failPrompts(count int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failCount = count
	c.promptErr = err
}

func (c *scriptedAuthACPClient) promptCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.prompts
}

func (c *scriptedAuthACPClient) authenticateCalls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.authCalls...)
}

// wireRecordingTransport is a real ACP transport double: it answers the
// handshake with a v2 result and records every method Matrix puts on the wire, so
// a test can assert what was never sent.
type wireRecordingTransport struct {
	mu        sync.Mutex
	methods   []string
	responses chan []byte
}

func newWireRecordingTransport() *wireRecordingTransport {
	return &wireRecordingTransport{responses: make(chan []byte, 8)}
}

func (t *wireRecordingTransport) Send(_ context.Context, message []byte) error {
	var req testJSONRPCRequest
	if err := json.Unmarshal(message, &req); err != nil {
		return err
	}
	t.mu.Lock()
	t.methods = append(t.methods, req.Method)
	t.mu.Unlock()
	if req.Method != "initialize" {
		return nil
	}
	payload, err := json.Marshal(testJSONRPCResponse{
		JSONRPC: "2.0",
		ID:      &req.ID,
		Result:  json.RawMessage(`{"protocolVersion":2,"authMethods":[{"methodId":"terminal-login","type":"terminal","name":"Terminal login"}]}`),
	})
	if err != nil {
		return err
	}
	t.responses <- payload
	return nil
}

func (t *wireRecordingTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case message := <-t.responses:
		return message, nil
	}
}

func (t *wireRecordingTransport) Close() error { return nil }

func (t *wireRecordingTransport) sentMethods() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.methods...)
}

// stubTransport is enough for the factory: the stubbed SDK never reads from it.
type stubTransport struct{}

func (stubTransport) Send(context.Context, []byte) error { return nil }
func (stubTransport) Receive(context.Context) ([]byte, error) {
	return nil, errors.New("stub transport")
}
func (stubTransport) Close() error { return nil }

type stubACPSDK struct{ client acpClient }

func (s stubACPSDK) NewClient(context.Context, middleware.AgentTransport) acpClient {
	return s.client
}

func authenticationRequiredError() error {
	return &zedacp.RPCError{
		Code:    -32000,
		Message: "authentication required",
		Data:    map[string]any{"kind": "auth_required"},
	}
}

func terminalMethod() acpAuthMethod {
	return acpAuthMethod{
		Type:     zedacp.AuthMethodTypeTerminal,
		MethodID: "terminal-login",
		Name:     "Log in from the terminal",
		Args:     []string{"--login"},
	}
}

// ----------------------------------------------------------------------------
// Offering methods
// ----------------------------------------------------------------------------

// TestAuthenticationMethodsOffersTerminalMethodsOnlyOnANegotiatedV2Connection
// is the offer rules in one table: a v2 connection that opted in offers the
// terminal method, a v1 connection never does, and custom "_" types are reported
// generically while unknown non-underscore types stay out of the list.
func TestAuthenticationMethodsOffersTerminalMethodsOnlyOnANegotiatedV2Connection(t *testing.T) {
	methods := []acpAuthMethod{
		{Type: zedacp.AuthMethodTypeAgent, MethodID: "agent-login", Name: "Agent login"},
		terminalMethod(),
		{Type: "_acme_sso", MethodID: "acme", Name: "Acme SSO"},
		{Type: "future_flow", MethodID: "future", Name: "Reserved for a later ACP version"},
	}
	cases := []struct {
		name         string
		version      int
		deps         middleware.ConversationFactoryDeps
		wantTerminal bool
	}{
		{
			name: "v2 with the opt-in and a process backend", version: zedacp.ProtocolVersionV2,
			deps: middleware.ConversationFactoryDeps{TerminalAuth: true, Process: &recordingProcess{}}, wantTerminal: true,
		},
		{
			name: "v1 never offers a terminal method", version: zedacp.ProtocolVersionV1,
			deps: middleware.ConversationFactoryDeps{TerminalAuth: true, Process: &recordingProcess{}},
		},
		{
			name: "v2 without the opt-in", version: zedacp.ProtocolVersionV2,
			deps: middleware.ConversationFactoryDeps{Process: &recordingProcess{}},
		},
		{
			name: "v2 without a process backend", version: zedacp.ProtocolVersionV2,
			deps: middleware.ConversationFactoryDeps{TerminalAuth: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &acpConversationClient{
				client:      newScriptedAuthACPClient(tc.version),
				deps:        tc.deps,
				authMethods: methods,
			}
			offered := map[string]string{}
			for _, method := range client.AuthenticationMethods() {
				offered[method.ID] = method.Type
			}
			if _, ok := offered["terminal-login"]; ok != tc.wantTerminal {
				t.Fatalf("terminal offered=%v, want %v (%#v)", ok, tc.wantTerminal, offered)
			}
			if offered["agent-login"] != zedacp.AuthMethodTypeAgent {
				t.Fatalf("agent method must always be offered: %#v", offered)
			}
			if offered["acme"] != "_acme_sso" {
				t.Fatalf("a custom type must be reported as declared: %#v", offered)
			}
			if _, ok := offered["future"]; ok {
				t.Fatalf("an unknown non-underscore type is reserved and must stay out: %#v", offered)
			}
		})
	}
}

// TestAuthenticationRefusesMethodsMatrixMustNotRun covers the fail-closed half:
// a custom type is advertised but cannot be run, an unknown type is not even
// advertised, and a terminal method on a v1 connection is not offered at all.
func TestAuthenticationRefusesMethodsMatrixMustNotRun(t *testing.T) {
	cases := []struct {
		name         string
		version      int
		terminalAuth bool
		call         string
		message      string
	}{
		{
			name: "custom type", version: zedacp.ProtocolVersionV2,
			call: "acme", message: "cannot run",
		},
		{
			name: "unknown reserved type", version: zedacp.ProtocolVersionV2,
			call: "future", message: "does not advertise",
		},
		{
			name: "terminal method on a v1 connection", version: zedacp.ProtocolVersionV1, terminalAuth: true,
			call: "terminal-login", message: "does not advertise",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proc := &recordingProcess{}
			client := &acpConversationClient{
				client: newScriptedAuthACPClient(tc.version),
				deps:   middleware.ConversationFactoryDeps{TerminalAuth: tc.terminalAuth, Process: proc},
				endpoint: middleware.ProtocolEndpoint{
					Command: "/opt/codex-acp",
				},
				authMethods: []acpAuthMethod{terminalMethod(), {Type: "_acme_sso", MethodID: "acme"}, {Type: "future_flow", MethodID: "future"}},
			}
			err := client.Authenticate(context.Background(), tc.call)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("expected %q refusal, got %v", tc.message, err)
			}
			if len(proc.specs) != 0 {
				t.Fatalf("a refused method must not launch anything: %#v", proc.specs)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Running a terminal method
// ----------------------------------------------------------------------------

// TestTerminalAuthenticationLaunchesTheConfiguredProgramWithMethodArgsAndEnv
// pins the launch contract: the same program and base configuration as the ACP
// connection, the method's args appended, and the method's env overriding a
// same-named base variable.
func TestTerminalAuthenticationLaunchesTheConfiguredProgramWithMethodArgsAndEnv(t *testing.T) {
	proc := &recordingProcess{}
	reconnects := 0
	client := &acpConversationClient{
		client: newScriptedAuthACPClient(zedacp.ProtocolVersionV2),
		deps:   middleware.ConversationFactoryDeps{AgentID: "codex", TerminalAuth: true, Process: proc},
		endpoint: middleware.ProtocolEndpoint{
			Command: "/opt/codex-acp",
			Args:    []string{"--experimental"},
			Env:     []string{"PATH=/usr/bin", "ACP_TOKEN=base"},
		},
		authMethods: []acpAuthMethod{{
			Type:     zedacp.AuthMethodTypeTerminal,
			MethodID: "terminal-login",
			Args:     []string{"--login"},
			Env: []zedacp.EnvVar{
				{Name: "ACP_TOKEN", Value: "login"},
				{Name: "ACP_INTERACTIVE_LOGIN", Value: "1"},
			},
		}},
		reconnect: func(context.Context) error { reconnects++; return nil },
	}

	if err := client.Authenticate(context.Background(), "terminal-login"); err != nil {
		t.Fatalf("terminal authentication failed: %v", err)
	}
	if len(proc.specs) != 1 {
		t.Fatalf("expected exactly one launched process, got %#v", proc.specs)
	}
	spec := proc.specs[0]
	if spec.Runner != "/opt/codex-acp" {
		t.Fatalf("the configured agent program must be launched, got %q", spec.Runner)
	}
	if got := strings.Join(spec.Args, " "); got != "--experimental --login" {
		t.Fatalf("base args plus method args, got %q", got)
	}
	env := strings.Join(spec.Env, " ")
	if !strings.Contains(env, "PATH=/usr/bin") {
		t.Fatalf("base env lost: %#v", spec.Env)
	}
	if !strings.Contains(env, "ACP_TOKEN=login") || strings.Contains(env, "ACP_TOKEN=base") {
		t.Fatalf("a same-named method variable must override the base value: %#v", spec.Env)
	}
	if !strings.Contains(env, "ACP_INTERACTIVE_LOGIN=1") {
		t.Fatalf("method env lost: %#v", spec.Env)
	}
	if reconnects != 1 {
		t.Fatalf("a successful terminal login must reconnect exactly once, got %d", reconnects)
	}
}

// TestTerminalAuthenticationReportsEveryFailureShapeAndDoesNotReconnect covers
// the exit rules: only exit status zero is success. Every failure has to be
// reported, and none of them may be followed by a reconnection.
func TestTerminalAuthenticationReportsEveryFailureShapeAndDoesNotReconnect(t *testing.T) {
	cases := []struct {
		name    string
		proc    *recordingProcess
		message string
	}{
		{name: "non-zero exit status", proc: &recordingProcess{exitCode: 7}, message: "exited with status 7"},
		{name: "termination without an exit status", proc: &recordingProcess{exitCode: -1}, message: "exited with status -1"},
		{name: "cancellation", proc: &recordingProcess{runErr: context.Canceled}, message: "failed to run"},
		{name: "launch failure", proc: &recordingProcess{runErr: errors.New("exec denied")}, message: "failed to run"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reconnects := 0
			client := &acpConversationClient{
				client:      newScriptedAuthACPClient(zedacp.ProtocolVersionV2),
				deps:        middleware.ConversationFactoryDeps{AgentID: "codex", TerminalAuth: true, Process: tc.proc},
				endpoint:    middleware.ProtocolEndpoint{Command: "/opt/codex-acp"},
				authMethods: []acpAuthMethod{terminalMethod()},
				reconnect:   func(context.Context) error { reconnects++; return nil },
			}
			err := client.Authenticate(context.Background(), "terminal-login")
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("expected %q, got %v", tc.message, err)
			}
			if reconnects != 0 {
				t.Fatalf("a failed terminal login must not reconnect, got %d", reconnects)
			}
		})
	}
}

// TestTerminalAuthenticationNeverSendsAuthLogin asserts the wire: a terminal
// method is completed by the launched process, so the ACP connection must not
// receive an auth/login — or anything else — for it.
func TestTerminalAuthenticationNeverSendsAuthLogin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport := newWireRecordingTransport()
	acpClient := zedacp.NewClient(ctx, transport)
	defer acpClient.Close()
	if _, err := acpClient.Initialize(ctx, acpInitializeRequest{ProtocolVersion: zedacp.ProtocolVersionV2}); err != nil {
		t.Fatalf("v2 initialize over the recording transport: %v", err)
	}

	reconnects := 0
	conversation := &acpConversationClient{
		client:      acpClient,
		deps:        middleware.ConversationFactoryDeps{AgentID: "codex", TerminalAuth: true, Process: &recordingProcess{}},
		endpoint:    middleware.ProtocolEndpoint{Command: "/opt/codex-acp"},
		authMethods: []acpAuthMethod{terminalMethod()},
		reconnect:   func(context.Context) error { reconnects++; return nil },
	}
	if err := conversation.Authenticate(ctx, "terminal-login"); err != nil {
		t.Fatalf("terminal authentication failed: %v", err)
	}
	if reconnects != 1 {
		t.Fatalf("a successful terminal login must reconnect, got %d", reconnects)
	}
	if methods := transport.sentMethods(); len(methods) != 1 || methods[0] != "initialize" {
		t.Fatalf("a terminal login must never reach the agent with a request: sent %v", methods)
	}
}

// TestTerminalLoginReconnectsBeforeRetryingTheGatedOperation is the ordering
// proof for the flow ACP prescribes: the gated turn fails, the terminal login
// runs, the connection is reinitialized, and only then is the turn retried — on
// the new connection.
func TestTerminalLoginReconnectsBeforeRetryingTheGatedOperation(t *testing.T) {
	events := []string{}
	gated := newScriptedAuthACPClient(zedacp.ProtocolVersionV2)
	gated.failPrompts(99, authenticationRequiredError())
	gated.onPrompt = func() { events = append(events, "prompt-gated") }
	healthy := newScriptedAuthACPClient(zedacp.ProtocolVersionV2)
	healthy.onPrompt = func() { events = append(events, "prompt-after-login") }
	proc := &recordingProcess{onRun: func() { events = append(events, "terminal-login") }}

	conversation := &acpConversationClient{
		client:         gated,
		deps:           middleware.ConversationFactoryDeps{AgentID: "codex", TerminalAuth: true, Process: proc},
		endpoint:       middleware.ProtocolEndpoint{Command: "/opt/codex-acp"},
		authMethods:    []acpAuthMethod{terminalMethod()},
		loadedSessions: map[string]bool{},
	}
	conversation.reconnect = func(context.Context) error {
		events = append(events, "reconnect")
		conversation.replaceACPClient(healthy, nil)
		return nil
	}

	_, err := conversation.ExecuteTurn(context.Background(), middleware.ConversationTurn{AgentID: "codex", Message: "hello"})
	if err != nil {
		t.Fatalf("the retried turn must succeed on the reconnected client: %v", err)
	}
	if want := "prompt-gated,terminal-login,reconnect,prompt-after-login"; strings.Join(events, ",") != want {
		t.Fatalf("flow order = %q, want %q", strings.Join(events, ","), want)
	}
	if conversation.currentACPClient() != healthy {
		t.Fatal("the retry must run on the reconnected connection")
	}
	if healthy.promptCount() != 1 || gated.promptCount() != 1 {
		t.Fatalf("expected one prompt per connection, gated=%d healthy=%d", gated.promptCount(), healthy.promptCount())
	}
}

// ----------------------------------------------------------------------------
// The structured auth_required retry
// ----------------------------------------------------------------------------

// TestAuthenticationRequiredLeadsToOneLoginAndOneRetry pins the bound: one login
// attempt, one retry, and a second failure that stays visible to the caller with
// the original structured signal still wrapped inside it.
func TestAuthenticationRequiredLeadsToOneLoginAndOneRetry(t *testing.T) {
	gated := newScriptedAuthACPClient(zedacp.ProtocolVersionV1)
	gated.failPrompts(99, authenticationRequiredError())
	conversation := &acpConversationClient{
		client:         gated,
		deps:           middleware.ConversationFactoryDeps{AgentID: "codex"},
		endpoint:       middleware.ProtocolEndpoint{Command: "/opt/codex-acp"},
		authMethods:    []acpAuthMethod{{Type: zedacp.AuthMethodTypeAgent, MethodID: "agent-login"}},
		loadedSessions: map[string]bool{},
	}

	_, err := conversation.ExecuteTurn(context.Background(), middleware.ConversationTurn{AgentID: "codex", Message: "hello"})
	if err == nil {
		t.Fatal("a second failure must be surfaced, not retried away")
	}
	if !strings.Contains(err.Error(), "retry after authentication") {
		t.Fatalf("the second failure must be identifiable as the retry's: %v", err)
	}
	if !zedacp.IsAuthenticationRequired(err) {
		t.Fatalf("the structured signal must stay wrapped: %v", err)
	}
	if calls := gated.authenticateCalls(); len(calls) != 1 || calls[0] != "agent-login" {
		t.Fatalf("expected exactly one login attempt, got %v", calls)
	}
	if count := gated.promptCount(); count != 2 {
		t.Fatalf("expected one retry after the login, got %d prompts", count)
	}
}

// TestAuthenticationRequiredRetriesOnceAndSucceeds is the other half of the same
// flow: when the login fixes the gate, the caller sees a normal turn result.
func TestAuthenticationRequiredRetriesOnceAndSucceeds(t *testing.T) {
	gated := newScriptedAuthACPClient(zedacp.ProtocolVersionV1)
	gated.failPrompts(1, authenticationRequiredError())
	promptUpdates := []acpSessionNotification{{
		SessionID: "remote-new",
		Update:    acpSessionUpdate{SessionUpdate: "agent_message_chunk", Content: acpContent{Type: "text", Text: "authenticated"}},
	}}
	gated.promptUpdates = promptUpdates
	conversation := &acpConversationClient{
		client:         gated,
		deps:           middleware.ConversationFactoryDeps{AgentID: "codex"},
		endpoint:       middleware.ProtocolEndpoint{Command: "/opt/codex-acp"},
		authMethods:    []acpAuthMethod{{Type: zedacp.AuthMethodTypeAgent, MethodID: "agent-login"}},
		loadedSessions: map[string]bool{},
	}

	result, err := conversation.ExecuteTurn(context.Background(), middleware.ConversationTurn{AgentID: "codex", Message: "hello"})
	if err != nil {
		t.Fatalf("the retried turn must succeed: %v", err)
	}
	if result.Output != "authenticated" {
		t.Fatalf("the retried turn output was lost: %q", result.Output)
	}
	if calls := gated.authenticateCalls(); len(calls) != 1 {
		t.Fatalf("expected exactly one login attempt, got %v", calls)
	}
	if count := gated.promptCount(); count != 2 {
		t.Fatalf("expected exactly one retry, got %d prompts", count)
	}
}

// TestAuthenticationRequiredWithoutARunnableMethodStaysAFailure: when the agent
// advertises nothing Matrix can run without a user, the gate is reported instead
// of being silently swallowed.
func TestAuthenticationRequiredWithoutARunnableMethodStaysAFailure(t *testing.T) {
	gated := newScriptedAuthACPClient(zedacp.ProtocolVersionV2)
	gated.failPrompts(99, authenticationRequiredError())
	conversation := &acpConversationClient{
		client:         gated,
		deps:           middleware.ConversationFactoryDeps{AgentID: "codex"},
		endpoint:       middleware.ProtocolEndpoint{Command: "/opt/codex-acp"},
		authMethods:    []acpAuthMethod{terminalMethod()},
		loadedSessions: map[string]bool{},
	}

	_, err := conversation.ExecuteTurn(context.Background(), middleware.ConversationTurn{AgentID: "codex", Message: "hello"})
	if err == nil || !strings.Contains(err.Error(), "no authentication method") {
		t.Fatalf("expected a visible no-method failure, got %v", err)
	}
	if count := gated.promptCount(); count != 1 {
		t.Fatalf("a request with no runnable method must not be retried, got %d prompts", count)
	}
}

// ----------------------------------------------------------------------------
// The initialize-time capability, and the factory that decides it
// ----------------------------------------------------------------------------

// TestTerminalAuthCapabilityIsAbsentUnlessTheOperatorOptsIn pins the opt-in: the
// advertisement and the runnable surface follow the same predicate, so Matrix
// never claims a flow it would refuse to run.
func TestTerminalAuthCapabilityIsAbsentUnlessTheOperatorOptsIn(t *testing.T) {
	cases := []struct {
		name string
		deps middleware.ConversationFactoryDeps
		want bool
	}{
		{name: "default", deps: middleware.ConversationFactoryDeps{Process: &recordingProcess{}}},
		{name: "opt-in without a process backend", deps: middleware.ConversationFactoryDeps{TerminalAuth: true}},
		{name: "process backend without the opt-in", deps: middleware.ConversationFactoryDeps{Process: &recordingProcess{}}},
		{name: "opt-in with a process backend", deps: middleware.ConversationFactoryDeps{TerminalAuth: true, Process: &recordingProcess{}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caps := acpClientCapabilitiesForDeps(tc.deps)
			advertised := caps.Auth != nil && caps.Auth.Terminal != nil
			if advertised != tc.want {
				t.Fatalf("capabilities.auth.terminal advertised=%v, want %v (%#v)", advertised, tc.want, caps.Auth)
			}
			encoded, err := json.Marshal(caps)
			if err != nil {
				t.Fatalf("marshal client capabilities: %v", err)
			}
			if onWire := strings.Contains(string(encoded), `"auth":{"terminal":{}}`); onWire != tc.want {
				t.Fatalf("wire advertisement=%v, want %v: %s", onWire, tc.want, encoded)
			}
		})
	}
}

// TestRouterTerminalAuthOptInReachesTheAdapter proves the production path: the
// switch set on the router is what the adapter reads, so enabling the surface
// cannot silently miss the ACP layer.
func TestRouterTerminalAuthOptInReachesTheAdapter(t *testing.T) {
	router := NewRouter(nil)
	router.SetProcess(&recordingProcess{})
	deps := middleware.ConversationFactoryDeps{AgentID: "codex", Process: router.proc, TerminalAuth: router.terminalAuth}
	if caps := acpClientCapabilitiesForDeps(deps); caps.Auth != nil {
		t.Fatalf("a fresh router must not advertise terminal authentication: %#v", caps.Auth)
	}

	router.SetTerminalAuth(true)
	deps.TerminalAuth = router.terminalAuth
	caps := acpClientCapabilitiesForDeps(deps)
	if caps.Auth == nil || caps.Auth.Terminal == nil {
		t.Fatal("the wired opt-in must reach the adapter as capabilities.auth.terminal")
	}
}

// TestInitializeConversationAcceptsANegotiatedV2Connection is the connection
// gate: a v2 agent used to be rejected outright, which made every v2 surface
// unreachable no matter what the client negotiated.
func TestInitializeConversationAcceptsANegotiatedV2Connection(t *testing.T) {
	fake := newScriptedAuthACPClient(zedacp.ProtocolVersionV2)
	fake.initializeResponse = &acpInitializeResponse{
		ProtocolVersion: zedacp.ProtocolVersionV2,
		AuthMethods: []acpAuthMethod{
			{Type: zedacp.AuthMethodTypeAgent, MethodID: "agent-login", Name: "Agent login"},
			terminalMethod(),
		},
	}
	previous := defaultACPSDK
	defaultACPSDK = stubACPSDK{client: fake}
	defer func() { defaultACPSDK = previous }()

	factory := &acpConversationFactory{}
	endpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: "/opt/codex-acp"}
	deps := middleware.ConversationFactoryDeps{AgentID: "codex", Cwd: "/workspace", TerminalAuth: true, Process: &recordingProcess{}}
	conversation, err := factory.initializeConversation(context.Background(), endpoint, deps, stubTransport{})
	if err != nil {
		t.Fatalf("an agent that negotiated v2 must be able to connect: %v", err)
	}
	control, ok := conversation.(middleware.ConversationAuthenticationControl)
	if !ok {
		t.Fatal("the conversation client must expose authentication control")
	}
	offered := map[string]string{}
	for _, method := range control.AuthenticationMethods() {
		offered[method.ID] = method.Type
	}
	if offered["terminal-login"] != zedacp.AuthMethodTypeTerminal {
		t.Fatalf("a v2 terminal method must be offered: %#v", offered)
	}

	// The same agent without the opt-in keeps the terminal method out.
	deps.TerminalAuth = false
	conversation, err = factory.initializeConversation(context.Background(), endpoint, deps, stubTransport{})
	if err != nil {
		t.Fatalf("initialize without the opt-in: %v", err)
	}
	control, ok = conversation.(middleware.ConversationAuthenticationControl)
	if !ok {
		t.Fatal("the conversation client must expose authentication control")
	}
	for _, method := range control.AuthenticationMethods() {
		if method.ID == "terminal-login" {
			t.Fatal("without the opt-in the terminal method must not be offered")
		}
	}
}

// TestInitializeConversationRejectsAnUnsupportedNegotiatedVersion keeps the gate
// fail-closed for a generation Matrix cannot speak.
func TestInitializeConversationRejectsAnUnsupportedNegotiatedVersion(t *testing.T) {
	fake := newScriptedAuthACPClient(zedacp.ProtocolVersionV2 + 1)
	previous := defaultACPSDK
	defaultACPSDK = stubACPSDK{client: fake}
	defer func() { defaultACPSDK = previous }()

	factory := &acpConversationFactory{}
	endpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: "/opt/codex-acp"}
	_, err := factory.initializeConversation(context.Background(), endpoint, middleware.ConversationFactoryDeps{Cwd: "/workspace"}, stubTransport{})
	if err == nil || !strings.Contains(err.Error(), "is not supported") {
		t.Fatalf("an unsupported negotiated version must fail closed, got %v", err)
	}
}
