package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
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
// ACP v2 terminal authentication against a real peer process
//
// The authentication unit tests in internal/providers/agents drive in-process
// fakes, so nothing there proves what the adapter puts on the wire or that a
// terminal method really launches a program. These tests drive the real adapter
// (agents.Router) against the repository's own mock agent running as a version 2
// peer over real stdio, and assert on what that peer observed rather than on
// what a fake was configured to expect:
//
//   - the handshake reaches version 2 with the parameter names v2 defined, and
//     the peer refuses the v1 names outright (the mock's rejection is exercised
//     directly as well, so the assertion has teeth);
//   - Matrix offers the terminal method the peer advertises;
//   - running the method really launches the configured program: the mock's
//     login mode leaves a credential file containing the env value the test
//     chose and the full argument vector, which is the only way to prove the
//     env override and the args merge reached a process;
//   - the gated prompt fails, the connection is reinitialized with a fresh
//     initialize, and the retry succeeds on that new process;
//   - no auth/login is ever sent for the terminal method.
//
// The peer is the repository's own binary built by buildMockACPAgent, so these
// tests need no network and no installed agent.
// ----------------------------------------------------------------------------

// The names below are the contract with cmd/mock-agent/acp_v2.go. They are
// repeated here on purpose: a rename that only lands on one side has to fail the
// test rather than move both ends silently.
const (
	mockV2Flag         = "--acp-v2"
	mockTerminalFlag   = "--terminal-login"
	mockTerminalMethod = "terminal-login"
	mockCredentialEnv  = "MOCK_AGENT_CREDENTIAL_PATH"
	mockMethodTokenEnv = "MOCK_AGENT_V2_METHOD_TOKEN"
	mockBaseTokenEnv   = "MOCK_AGENT_TERMINAL_TOKEN"
	mockLogEnv         = "MOCK_AGENT_LOG_PATH"
	mockAcceptedText   = "v2 prompt accepted"
	mockV2AgentID      = "mock-agent-v2"
)

// acpV2MockResolver publishes the mock agent as the endpoint the router uses.
type acpV2MockResolver struct {
	bin  string
	args []string
	env  []string
}

func (r *acpV2MockResolver) GetAgentEndpoint(_ string) (middleware.ProtocolEndpoint, error) {
	return middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "stdio",
		Command:   r.bin,
		Args:      append([]string{}, r.args...),
		Env:       append([]string{}, r.env...),
	}, nil
}

// mockPeerRecord is one line of the observation file the v2 peer appends to.
type mockPeerRecord struct {
	Kind    string                 `json:"kind"`
	Method  string                 `json:"method"`
	PID     int                    `json:"pid"`
	Args    []string               `json:"args"`
	Details map[string]interface{} `json:"details"`
}

// terminalLoginCredential is the file the login program leaves behind.
type terminalLoginCredential struct {
	Token string   `json:"token"`
	Args  []string `json:"args"`
	PID   int      `json:"pid"`
}

// TestACPv2TerminalAuthenticationOverRealStdioProcess is the end-to-end proof:
// a v2 handshake over real stdio, a terminal method that really runs a program,
// a reconnect, and a gated prompt that succeeds only afterwards.
func TestACPv2TerminalAuthenticationOverRealStdioProcess(t *testing.T) {
	requireMockAgentBuild(t)
	bin := buildMockACPAgent(t)
	workspace := t.TempDir()
	evidenceDir := t.TempDir()
	logPath := filepath.Join(evidenceDir, "peer-methods.jsonl")
	credentialPath := filepath.Join(evidenceDir, "terminal-credential.json")
	baseToken := "stale-base-token-" + randomToken(t)
	methodToken := "method-token-" + randomToken(t)

	resolver := &acpV2MockResolver{
		bin:  bin,
		args: []string{"--cwd", workspace, mockV2Flag},
		env: []string{
			mockLogEnv + "=" + logPath,
			mockCredentialEnv + "=" + credentialPath,
			// The base launch configuration carries a stale value for the variable
			// the method overrides, so the credential proves which one won.
			mockBaseTokenEnv + "=" + baseToken,
			mockMethodTokenEnv + "=" + methodToken,
		},
	}
	router := agents.NewRouter(resolver)
	router.SetTrustMode(func() bool { return true })
	router.SetProcess(execprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), workspace)
	// The operator opt-in: without it neither the capability nor the method is
	// offered, and this test would silently degrade into a non-v2 flow.
	router.SetTerminalAuth(true)
	defer router.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// The login program has not run yet, so the credential cannot exist.
	if _, err := os.Stat(credentialPath); err == nil {
		t.Fatalf("the credential file already exists before any authentication: %s", credentialPath)
	}

	methods, err := router.AgentAuthenticationMethods(ctx, mockV2AgentID)
	if err != nil {
		t.Fatalf("Matrix did not report the v2 peer's authentication methods: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("the peer advertises exactly one runnable method, Matrix offered %+v", methods)
	}
	if methods[0].Type != zedacp.AuthMethodTypeTerminal || methods[0].ID != mockTerminalMethod {
		t.Fatalf("Matrix must offer the peer's terminal method, got %+v", methods[0])
	}

	// This prompt is gated by the peer until the login has happened. The adapter
	// has to run the terminal method, reconnect and retry for it to succeed.
	output, remoteSessionID, _, _, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          mockV2AgentID,
		LogicalSessionID: "acp-v2-terminal-auth",
		WorkspacePath:    workspace,
		Message:          "hello from the v2 terminal authentication test",
	})
	if err != nil {
		t.Fatalf("the gated prompt never succeeded after terminal authentication: %v", err)
	}
	if remoteSessionID == "" {
		t.Fatal("the retried turn returned no remote session id")
	}
	if !strings.Contains(output, mockAcceptedText) {
		t.Fatalf("the retried prompt must have been answered by the authenticated peer, output=%q", output)
	}

	records := readMockPeerLog(t, logPath)
	assertV2HandshakeOverTheWire(t, records)
	assertTerminalLoginRanAsARealProcess(t, records, credentialPath, baseToken, methodToken, workspace)
	assertGatedPromptWasRetriedOnAReinitializedConnection(t, records)
	assertNoWireLoginForATerminalMethod(t, records)
}

// TestACPv2MockPeerRejectsV1InitializeParameterNamesAndAcceptsV2Names is the
// teeth of the handshake assertion. It talks to the same real peer with the
// names v1 defined and with the names v2 defined, and shows that only the second
// completes the handshake: if InitializeRequest.MarshalJSON regressed to the v1
// names for a v2 request, the main test could not reach v2 at all.
func TestACPv2MockPeerRejectsV1InitializeParameterNamesAndAcceptsV2Names(t *testing.T) {
	requireMockAgentBuild(t)
	bin := buildMockACPAgent(t)
	evidenceDir := t.TempDir()
	logPath := filepath.Join(evidenceDir, "peer-methods.jsonl")
	env := []string{
		mockLogEnv + "=" + logPath,
		mockCredentialEnv + "=" + filepath.Join(evidenceDir, "terminal-credential.json"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// A version 2 request that still carries version 1's parameter names. The
	// peer implements the specification: it reads capabilities and info, so the
	// v1 names advertise nothing and the handshake is refused as invalid params.
	wrongNames := newRawMockV2Peer(ctx, t, bin, env)
	defer wrongNames.Close()
	var refused map[string]interface{}
	err := wrongNames.ExtRequest(ctx, "initialize", map[string]interface{}{
		"protocolVersion":    2,
		"clientInfo":         map[string]interface{}{"name": "matrix-v1-wire-names", "version": "0.0.0-test"},
		"clientCapabilities": map[string]interface{}{},
	}, &refused)
	if err == nil {
		t.Fatalf("the v2 peer accepted v1 parameter names and answered %v", refused)
	}
	var rpcErr *zedacp.RPCError
	if !errors.As(err, &rpcErr) || rpcErr == nil {
		t.Fatalf("the refusal must be a typed JSON-RPC error, not a warning: %#v", err)
	}
	if rpcErr.Code != -32602 {
		t.Fatalf("the refusal must be invalid params, got %+v", rpcErr)
	}
	t.Logf("v1-named v2 initialize was refused with: %v", err)

	// The control on the same binary: the v2 names complete the handshake, so
	// the refusal above is about the parameter names and nothing else.
	rightNames := newRawMockV2Peer(ctx, t, bin, env)
	defer rightNames.Close()
	initResp, err := rightNames.Initialize(ctx, zedacp.InitializeRequest{
		ProtocolVersion: zedacp.ProtocolVersionV2,
		ClientInfo:      map[string]interface{}{"name": "matrix-v2-wire-names", "version": "0.0.0-test"},
		ClientCapabilities: &zedacp.ClientCapabilities{
			// The peer may offer a terminal method only when the client
			// advertised that it can run one, which is the shape Matrix sends
			// for a v2 connection with terminal authentication enabled.
			Auth: &zedacp.AuthCapabilities{Terminal: &zedacp.TerminalAuthCapabilities{}},
		},
	})
	if err != nil {
		t.Fatalf("the v2 peer refused the v2 parameter names: %v", err)
	}
	if initResp.ProtocolVersion != zedacp.ProtocolVersionV2 {
		t.Fatalf("the negotiated version must be 2, got %d", initResp.ProtocolVersion)
	}
	if len(initResp.AuthMethods) != 1 || initResp.AuthMethods[0].MethodType() != zedacp.AuthMethodTypeTerminal {
		t.Fatalf("the v2 handshake must advertise the terminal method, got %+v", initResp.AuthMethods)
	}

	records := readMockPeerLog(t, logPath)
	assertPeerRejectionRecordedTheV1Names(t, records)
}

// requireMockAgentBuild skips rather than fails when the environment cannot
// build the in-repo peer. Nothing here needs the network or an installed agent:
// the only requirement is the Go toolchain the package already builds with.
func requireMockAgentBuild(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("the Go toolchain needed to build the in-repo mock ACP peer is unavailable: %v", err)
	}
}

func newRawMockV2Peer(ctx context.Context, t *testing.T, bin string, env []string) *zedacp.Client {
	t.Helper()
	transport, err := zedacp.NewStdioTransport(ctx, bin, env, mockV2Flag)
	if err != nil {
		t.Fatalf("start the mock v2 peer: %v", err)
	}
	return zedacp.NewClient(ctx, transport)
}

// assertV2HandshakeOverTheWire checks the parameter names the peer actually
// received, not the ones the client intended to send.
func assertV2HandshakeOverTheWire(t *testing.T, records []mockPeerRecord) {
	t.Helper()
	handshakes := peerRecords(records, func(record mockPeerRecord) bool {
		return record.Kind == "initialize_seen"
	})
	if len(handshakes) < 2 {
		t.Fatalf("expected an initialize on the first connection and another after the reconnect, saw %d: %s",
			len(handshakes), describePeerRecords(records))
	}
	for _, handshake := range handshakes {
		if handshake.Details["accepted"] != true {
			t.Fatalf("every initialize this client sends must be accepted, got %s", describePeerRecord(handshake))
		}
		if handshake.Details["protocolVersion"] != float64(zedacp.ProtocolVersionV2) {
			t.Fatalf("the handshake must ask for v2, got %s", describePeerRecord(handshake))
		}
		if handshake.Details["hasCapabilities"] != true || handshake.Details["hasInfo"] != true {
			t.Fatalf("a v2 request must carry capabilities and info, got %s", describePeerRecord(handshake))
		}
		if handshake.Details["hasV1Capabilities"] != false || handshake.Details["hasV1Info"] != false {
			t.Fatalf("a v2 request must not carry the v1 parameter names, got %s", describePeerRecord(handshake))
		}
		if handshake.Details["authTerminalCapability"] != true {
			t.Fatalf("Matrix must advertise capabilities.auth.terminal, without which a v2 peer may not offer a terminal method: %s",
				describePeerRecord(handshake))
		}
		names := peerParamNames(handshake.Details)
		if !names["capabilities"] || !names["info"] {
			t.Fatalf("the request must carry the names v2 defined, got %s", describePeerRecord(handshake))
		}
	}
	if rejected := peerRecords(records, func(record mockPeerRecord) bool {
		return record.Kind == "initialize_rejected"
	}); len(rejected) != 0 {
		t.Fatalf("the client's own handshake must never be refused, the peer recorded %s", describePeerRecords(rejected))
	}
}

// assertPeerRejectionRecordedTheV1Names proves the refusal in the teeth test is
// about the names that arrived, and is answered as a failure rather than a
// warning.
func assertPeerRejectionRecordedTheV1Names(t *testing.T, records []mockPeerRecord) {
	t.Helper()
	v1Handshake := peerRecords(records, func(record mockPeerRecord) bool {
		return record.Kind == "initialize_seen" && record.Details["hasV1Info"] == true
	})
	if len(v1Handshake) != 1 {
		t.Fatalf("the peer must have recorded the v1-named handshake it saw, got %s", describePeerRecords(records))
	}
	if v1Handshake[0].Details["accepted"] != false {
		t.Fatalf("the v1-named handshake must be recorded as refused, got %s", describePeerRecord(v1Handshake[0]))
	}
	if v1Handshake[0].Details["hasV1Capabilities"] != true || v1Handshake[0].Details["hasInfo"] != false {
		t.Fatalf("the peer must record exactly the names that arrived, got %s", describePeerRecord(v1Handshake[0]))
	}
	rejections := peerRecords(records, func(record mockPeerRecord) bool {
		return record.Kind == "initialize_rejected"
	})
	if len(rejections) != 1 || !strings.Contains(strings.ToLower(asString(rejections[0].Details["reason"])), "version 2") {
		t.Fatalf("the refusal must be recorded with its reason, got %s", describePeerRecords(records))
	}
	accepted := peerRecords(records, func(record mockPeerRecord) bool {
		return record.Kind == "initialize_seen" && record.Details["accepted"] == true
	})
	if len(accepted) != 1 || accepted[0].Details["hasV1Info"] != false {
		t.Fatalf("the v2-named handshake must be the one that was accepted, got %s", describePeerRecords(records))
	}
}

// assertTerminalLoginRanAsARealProcess reads the marker the login program wrote.
// The env value in it is the one the method advertised (not the base launch
// configuration's same-named value) and the argument vector carries the base
// args with the method's appended, so both halves of the launch spec are proven
// to have reached a process rather than a struct.
func assertTerminalLoginRanAsARealProcess(t *testing.T, records []mockPeerRecord, credentialPath, baseToken, methodToken, workspace string) {
	t.Helper()
	raw, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatalf("the terminal method never left its credential file: %v", err)
	}
	var credential terminalLoginCredential
	if err := json.Unmarshal(raw, &credential); err != nil {
		t.Fatalf("the credential file is not the login program's record (%s): %v", raw, err)
	}
	if credential.Token != methodToken {
		t.Fatalf("the method's env must override the base launch configuration: token=%q want=%q (base was %q)",
			credential.Token, methodToken, baseToken)
	}
	for _, want := range []string{"--cwd", workspace, mockV2Flag, mockTerminalFlag} {
		if !containsStringValue(credential.Args, want) {
			t.Fatalf("the login program must be launched with the base args plus the method's, args=%v missing %q", credential.Args, want)
		}
	}
	logins := peerRecords(records, func(record mockPeerRecord) bool {
		return record.Kind == "terminal_login"
	})
	if len(logins) != 1 {
		t.Fatalf("expected exactly one terminal login process, got %s", describePeerRecords(records))
	}
	if logins[0].Details["token"] != methodToken {
		t.Fatalf("the peer's own record of the login must carry the method's env value, got %s", describePeerRecord(logins[0]))
	}
	prompts := peerRecords(records, func(record mockPeerRecord) bool { return record.Method == "session/prompt" })
	if len(prompts) == 0 || logins[0].PID == prompts[0].PID {
		t.Fatalf("the login must be a separate process from the ACP peer: login pid=%d prompt pid=%d",
			logins[0].PID, firstPID(prompts))
	}
}

// assertGatedPromptWasRetriedOnAReinitializedConnection orders the peer's own
// observations: the first prompt is refused while unauthenticated, the login
// runs, a new process initializes, and the retry is answered by that process.
func assertGatedPromptWasRetriedOnAReinitializedConnection(t *testing.T, records []mockPeerRecord) {
	t.Helper()
	promptIndices := indicesOfPeerRecords(records, func(record mockPeerRecord) bool { return record.Method == "session/prompt" })
	if len(promptIndices) != 2 {
		t.Fatalf("expected the gated prompt and exactly one retry, got %s", describePeerRecords(records))
	}
	gated := records[promptIndices[0]]
	retried := records[promptIndices[1]]
	if gated.Details["authenticated"] != false {
		t.Fatalf("the first prompt must arrive while the peer is unauthenticated, got %s", describePeerRecord(gated))
	}
	if retried.Details["authenticated"] != true {
		t.Fatalf("the retried prompt must arrive after the login, got %s", describePeerRecord(retried))
	}
	login := indexOfPeerRecord(records, func(record mockPeerRecord) bool { return record.Kind == "terminal_login" })
	reinitialize := indexOfPeerRecord(records, func(record mockPeerRecord) bool {
		return record.Kind == "initialize_seen" && record.PID == retried.PID
	})
	if login < 0 || reinitialize < 0 {
		t.Fatalf("the peer log is missing the login or the reinitialization: %s", describePeerRecords(records))
	}
	if promptIndices[0] >= login || login >= reinitialize || reinitialize >= promptIndices[1] {
		t.Fatalf("order must be gated prompt, login, reinitialize, retry; got indices %d, %d, %d, %d: %s",
			promptIndices[0], login, reinitialize, promptIndices[1], describePeerRecords(records))
	}
	if gated.PID == retried.PID {
		t.Fatalf("the retry must run on the reconnected process, both prompts came from pid %d", gated.PID)
	}
}

// assertNoWireLoginForATerminalMethod checks the method names the peer was sent.
// ACP v2 forbids auth/login for a terminal method, and the peer answers it with
// an error, so a client that sent one could not have completed the flow above.
func assertNoWireLoginForATerminalMethod(t *testing.T, records []mockPeerRecord) {
	t.Helper()
	for _, record := range records {
		if record.Method == "auth/login" || record.Method == "authenticate" {
			t.Fatalf("a terminal authentication method must never reach the wire as %q: %s",
				record.Method, describePeerRecord(record))
		}
	}
}

func peerRecords(records []mockPeerRecord, match func(mockPeerRecord) bool) []mockPeerRecord {
	out := make([]mockPeerRecord, 0, len(records))
	for _, record := range records {
		if match(record) {
			out = append(out, record)
		}
	}
	return out
}

func indicesOfPeerRecords(records []mockPeerRecord, match func(mockPeerRecord) bool) []int {
	out := make([]int, 0, len(records))
	for i, record := range records {
		if match(record) {
			out = append(out, i)
		}
	}
	return out
}

func indexOfPeerRecord(records []mockPeerRecord, match func(mockPeerRecord) bool) int {
	for i, record := range records {
		if match(record) {
			return i
		}
	}
	return -1
}

func firstPID(records []mockPeerRecord) int {
	if len(records) == 0 {
		return 0
	}
	return records[0].PID
}

func peerParamNames(details map[string]interface{}) map[string]bool {
	names := map[string]bool{}
	values, _ := details["paramNames"].([]interface{})
	for _, value := range values {
		if name, ok := value.(string); ok {
			names[name] = true
		}
	}
	return names
}

func containsStringValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func asString(value interface{}) string {
	text, _ := value.(string)
	return text
}

func readMockPeerLog(t *testing.T, path string) []mockPeerRecord {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the peer recorded nothing at %s: %v", path, err)
	}
	records := make([]mockPeerRecord, 0)
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record mockPeerRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("malformed peer record %q: %v", line, err)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		t.Fatalf("the peer recorded nothing at %s", path)
	}
	return records
}

func describePeerRecords(records []mockPeerRecord) string {
	parts := make([]string, 0, len(records))
	for _, record := range records {
		parts = append(parts, describePeerRecord(record))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func describePeerRecord(record mockPeerRecord) string {
	encoded, err := json.Marshal(record)
	if err != nil {
		return record.Kind + "/" + record.Method
	}
	return string(encoded)
}

func randomToken(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("generate a token: %v", err)
	}
	return hex.EncodeToString(buf)
}
