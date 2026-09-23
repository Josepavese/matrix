package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestV2Peer starts the version 2 peer in-process with an observation file
// and a credential path inside the test's temp dir.
func newTestV2Peer(t *testing.T) (*acpV2Peer, string, string) {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "methods.jsonl")
	credentialPath := filepath.Join(dir, "credential.json")
	t.Setenv(envPeerLogPath, logPath)
	t.Setenv(envCredentialPath, credentialPath)
	return newACPV2PeerFromArgs([]string{acpV2Flag}), logPath, credentialPath
}

func readPeerRecords(t *testing.T, path string) []peerLogRecord {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the peer recorded nothing at %s: %v", path, err)
	}
	records := make([]peerLogRecord, 0)
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record peerLogRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("malformed peer record %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func findPeerRecord(records []peerLogRecord, kind string) (peerLogRecord, bool) {
	for _, record := range records {
		if record.Kind == kind {
			return record, true
		}
	}
	return peerLogRecord{}, false
}

func initializeRequest(id int, params string) jsonRPCRequest {
	return jsonRPCRequest{ID: id, Method: "initialize", Params: json.RawMessage(params)}
}

func promptRequest(id int, text string) jsonRPCRequest {
	params, _ := json.Marshal(map[string]interface{}{
		"sessionId": "mock-session-id",
		"prompt":    []map[string]string{{"text": text}},
	})
	return jsonRPCRequest{ID: id, Method: "session/prompt", Params: json.RawMessage(params)}
}

// TestACPv2InitializeRejectsV1ParameterNamesAndRecordsThem is the refusal the
// real-peer interop test depends on: a version 2 request carrying version 1's
// parameter names is a failure, and the names that arrived are recorded so a
// caller can assert the wire shape instead of guessing it.
func TestACPv2InitializeRejectsV1ParameterNamesAndRecordsThem(t *testing.T) {
	peer, logPath, _ := newTestV2Peer(t)

	resp := peer.initialize(initializeRequest(1, `{"protocolVersion":2,"clientInfo":{"name":"legacy"},"clientCapabilities":{}}`))
	if resp.Error == nil {
		t.Fatalf("v1 parameter names must be refused, got result %s", resp.Result)
	}
	if resp.Error.Code != -32602 {
		t.Fatalf("the refusal must be invalid params, got %+v", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "capabilities and info") {
		t.Fatalf("the refusal must name what v2 requires, got %q", resp.Error.Message)
	}
	seen, ok := findPeerRecord(readPeerRecords(t, logPath), "initialize_seen")
	if !ok {
		t.Fatal("the peer must record the initialize it saw")
	}
	if seen.Details["accepted"] != false || seen.Details["hasV1Info"] != true || seen.Details["hasV1Capabilities"] != true {
		t.Fatalf("the record must show the names that arrived and the refusal: %+v", seen.Details)
	}
	if seen.Details["hasInfo"] != false || seen.Details["hasCapabilities"] != false {
		t.Fatalf("the record must not claim v2 names that never arrived: %+v", seen.Details)
	}
	if _, ok := findPeerRecord(readPeerRecords(t, logPath), "initialize_rejected"); !ok {
		t.Fatal("the refusal must also be recorded as its own event")
	}
}

// TestACPv2InitializeAcceptsV2NamesAndAdvertisesTheTerminalMethod pins the
// accepted handshake and the method payload the adapter has to reproduce: a
// terminal type carrying the args to append and the env to override, offered
// because the client advertised that it can run one.
func TestACPv2InitializeAcceptsV2NamesAndAdvertisesTheTerminalMethod(t *testing.T) {
	peer, logPath, _ := newTestV2Peer(t)
	t.Setenv(envMethodToken, "method-token-from-test")

	resp := peer.initialize(initializeRequest(2, `{"protocolVersion":2,"capabilities":{"auth":{"terminal":{}}},"info":{"name":"matrix"}}`))
	if resp.Error != nil {
		t.Fatalf("the v2 names must be accepted, got %+v", resp.Error)
	}
	var result struct {
		ProtocolVersion int `json:"protocolVersion"`
		AuthMethods     []struct {
			Type     string   `json:"type"`
			MethodID string   `json:"methodId"`
			Args     []string `json:"args"`
			Env      []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"env"`
		} `json:"authMethods"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if result.ProtocolVersion != 2 {
		t.Fatalf("the handshake must negotiate version 2, got %d", result.ProtocolVersion)
	}
	if len(result.AuthMethods) != 1 {
		t.Fatalf("exactly one method is advertised, got %+v", result.AuthMethods)
	}
	method := result.AuthMethods[0]
	if method.Type != "terminal" || method.MethodID != terminalMethodID {
		t.Fatalf("the method must be the terminal login, got %+v", method)
	}
	if !hasArg(method.Args, terminalLoginFlag) {
		t.Fatalf("the method must name the login flag, got args %v", method.Args)
	}
	if len(method.Env) != 1 || method.Env[0].Name != envBaseToken || method.Env[0].Value != "method-token-from-test" {
		t.Fatalf("the method must carry the token the test chose, got %+v", method.Env)
	}
	seen, ok := findPeerRecord(readPeerRecords(t, logPath), "initialize_seen")
	if !ok || seen.Details["accepted"] != true {
		t.Fatalf("the accepted handshake must be recorded: %+v %v", seen.Details, ok)
	}
	if seen.Details["authTerminalCapability"] != true {
		t.Fatalf("the record must show the capability the method was offered for: %+v", seen.Details)
	}
}

// TestACPv2InitializeOmitsTheTerminalMethodWhenTheClientCapabilityIsMissing
// keeps the offer conditional, which is what v2 requires: an agent may advertise
// a terminal method only when the client sent capabilities.auth.terminal, and a
// client that did not must still complete the handshake.
func TestACPv2InitializeOmitsTheTerminalMethodWhenTheClientCapabilityIsMissing(t *testing.T) {
	peer, logPath, _ := newTestV2Peer(t)

	resp := peer.initialize(initializeRequest(7, `{"protocolVersion":2,"capabilities":{},"info":{"name":"matrix"}}`))
	if resp.Error != nil {
		t.Fatalf("a client without the terminal capability must still handshake, got %+v", resp.Error)
	}
	var result struct {
		ProtocolVersion int               `json:"protocolVersion"`
		AuthMethods     []json.RawMessage `json:"authMethods"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if result.ProtocolVersion != 2 {
		t.Fatalf("the handshake must still negotiate version 2, got %d", result.ProtocolVersion)
	}
	if len(result.AuthMethods) != 0 {
		t.Fatalf("no terminal method may be advertised without the client capability, got %+v", result.AuthMethods)
	}
	seen, ok := findPeerRecord(readPeerRecords(t, logPath), "initialize_seen")
	if !ok || seen.Details["authTerminalCapability"] != false || seen.Details["accepted"] != true {
		t.Fatalf("the record must show an accepted handshake without the capability: %+v %v", seen.Details, ok)
	}
}

// TestACPv2RejectsWireLoginForATerminalMethod keeps the forbidden request from
// looking like a success: v2 completes a terminal method by running the program,
// so a wire login is answered with an error and recorded under its own name.
func TestACPv2RejectsWireLoginForATerminalMethod(t *testing.T) {
	peer, logPath, _ := newTestV2Peer(t)

	resp, _ := peer.handle(jsonRPCRequest{ID: 3, Method: "auth/login", Params: json.RawMessage(`{"methodId":"terminal-login"}`)})
	if resp.Error == nil {
		t.Fatalf("auth/login must not be accepted for a terminal method, got result %s", resp.Result)
	}
	if resp.Error.Code != -32601 {
		t.Fatalf("the forbidden login must be method-not-found, got %+v", resp.Error)
	}
	records := readPeerRecords(t, logPath)
	login, ok := findPeerRecord(records, "request")
	if !ok || login.Method != "auth/login" {
		t.Fatalf("the forbidden login must still be recorded as received: %+v %v", login, ok)
	}
}

// TestACPv2PromptIsGatedUntilAReconnectingProcessSeesTheCredential pins both
// halves of the gate: an unauthenticated process answers with the structured
// marker, and it stays unauthenticated even after the credential appears,
// because the gate is read at startup. That is what makes a reconnect necessary
// rather than optional.
func TestACPv2PromptIsGatedUntilAReconnectingProcessSeesTheCredential(t *testing.T) {
	peer, _, credentialPath := newTestV2Peer(t)

	gated, _ := peer.prompt(promptRequest(4, "hello"))
	if gated.Error == nil {
		t.Fatalf("an unauthenticated prompt must fail, got result %s", gated.Result)
	}
	if !strings.Contains(strings.ToLower(fmt.Sprint(gated.Error.Data)), "auth_required") {
		t.Fatalf("the failure must carry the structured marker, got %+v", gated.Error)
	}

	if err := os.WriteFile(credentialPath, []byte(`{"token":"late"}`), 0o600); err != nil {
		t.Fatalf("write the credential: %v", err)
	}
	if resp, _ := peer.prompt(promptRequest(5, "hello again")); resp.Error == nil {
		t.Fatal("the running process must stay unauthenticated until it is reconnected")
	}

	reconnected := newACPV2PeerFromArgs([]string{acpV2Flag})
	var accepted jsonRPCResponse
	output := captureStdout(t, func() {
		accepted, _ = reconnected.prompt(promptRequest(6, "hello after login"))
	})
	if accepted.Error != nil {
		t.Fatalf("a process that starts with the credential must accept the prompt, got %+v", accepted.Error)
	}
	if !strings.Contains(output, promptAcceptedText) {
		t.Fatalf("the authenticated prompt must answer with its marker, got:\n%s", output)
	}
}

// TestRunTerminalLoginWritesWhatItWasLaunchedWith pins the login program's
// observable marker: the env value it read and the argument vector it ran as.
func TestRunTerminalLoginWritesWhatItWasLaunchedWith(t *testing.T) {
	peer, logPath, credentialPath := newTestV2Peer(t)
	if peer == nil {
		t.Fatal("the v2 peer must be built from the flag")
	}
	t.Setenv(envBaseToken, "token-from-the-process-env")
	originalArgs := os.Args
	os.Args = []string{"/opt/mock-agent", acpV2Flag, terminalLoginFlag}
	t.Cleanup(func() { os.Args = originalArgs })

	if code := runTerminalLogin(); code != 0 {
		t.Fatalf("a successful login must exit zero, got %d", code)
	}
	raw, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatalf("the login must leave a credential: %v", err)
	}
	var record terminalLoginRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("decode credential: %v", err)
	}
	if record.Token != "token-from-the-process-env" {
		t.Fatalf("the credential must carry the env value the process read, got %q", record.Token)
	}
	if !hasArg(record.Args, terminalLoginFlag) {
		t.Fatalf("the credential must carry the argument vector, got %v", record.Args)
	}
	login, ok := findPeerRecord(readPeerRecords(t, logPath), "terminal_login")
	if !ok || login.Details["token"] != "token-from-the-process-env" {
		t.Fatalf("the login must also be recorded in order: %+v %v", login, ok)
	}
}

// TestTerminalLoginRequestedNeedsTheExactFlag keeps an ordinary v2 peer from
// being mistaken for the login program by a substring or a partial argument.
func TestTerminalLoginRequestedNeedsTheExactFlag(t *testing.T) {
	if !terminalLoginRequested([]string{"--cwd", "/tmp", acpV2Flag, terminalLoginFlag}) {
		t.Fatal("the exact flag must select the login program")
	}
	if terminalLoginRequested([]string{acpV2Flag, "--terminal-login=false"}) {
		t.Fatal("a different argument must not select the login program")
	}
	if terminalLoginRequested(nil) {
		t.Fatal("an empty argument vector must not select the login program")
	}
}

// ----------------------------------------------------------------------------
// The authentication types, the logout gate and the observed capability keys
// ----------------------------------------------------------------------------

// TestACPv2AgentLoginAuthenticatesTheRunningProcessAndLogoutGatesItAgain is the
// protocol-driven flow end to end at the peer: an agent-type method is completed
// by auth/login on the running process, the gated prompt then succeeds, and
// auth/logout puts the gate back — which is what the specification says happens
// to authentication-gated requests after a logout.
func TestACPv2AgentLoginAuthenticatesTheRunningProcessAndLogoutGatesItAgain(t *testing.T) {
	t.Setenv(envAuthType, authTypeAgent)
	peer, logPath, credentialPath := newTestV2Peer(t)

	resp := peer.initialize(initializeRequest(1, `{"protocolVersion":2,"capabilities":{},"info":{"name":"matrix"}}`))
	if resp.Error != nil {
		t.Fatalf("the handshake must be accepted: %+v", resp.Error)
	}
	var result struct {
		AuthMethods []struct {
			Type     string `json:"type"`
			MethodID string `json:"methodId"`
		} `json:"authMethods"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if len(result.AuthMethods) != 1 || result.AuthMethods[0].Type != authTypeAgent || result.AuthMethods[0].MethodID != agentMethodID {
		t.Fatalf("the agent type must advertise exactly the agent method, got %+v", result.AuthMethods)
	}

	if gated, _ := peer.prompt(promptRequest(2, "hello")); gated.Error == nil || gated.Error.Code != -32000 {
		t.Fatalf("an unauthenticated prompt must be gated with -32000, got %+v", gated)
	}

	login, _ := peer.handle(jsonRPCRequest{ID: 3, Method: "auth/login", Params: json.RawMessage(`{"methodId":"agent-login"}`)})
	if login.Error != nil {
		t.Fatalf("auth/login for the advertised agent method must succeed: %+v", login.Error)
	}
	if accepted, _ := peer.prompt(promptRequest(4, "hello again")); accepted.Error != nil {
		t.Fatalf("the process must be authenticated after the login, got %+v", accepted.Error)
	}

	logout, _ := peer.handle(jsonRPCRequest{ID: 5, Method: "auth/logout", Params: json.RawMessage(`{}`)})
	if logout.Error != nil {
		t.Fatalf("auth/logout must succeed while a method is advertised: %+v", logout.Error)
	}
	if gatedAgain, _ := peer.prompt(promptRequest(6, "hello after logout")); gatedAgain.Error == nil || gatedAgain.Error.Code != -32000 {
		t.Fatalf("the gate must come back after a logout, got %+v", gatedAgain)
	}

	if fileExists(credentialPath) {
		t.Fatal("the agent-handled flow must not leave a terminal credential")
	}
	records := readPeerRecords(t, logPath)
	methods := []string{}
	for _, record := range records {
		if record.Kind == "request" {
			methods = append(methods, record.Method)
		}
	}
	want := []string{"session/prompt", "auth/login", "session/prompt", "auth/logout", "session/prompt"}
	if strings.Join(methods, ",") != strings.Join(want, ",") {
		t.Fatalf("peer saw methods %v, want %v", methods, want)
	}
}

// TestACPv2GatedFailureCanBeTheSpecificationCodeAlone pins the shape the
// specification defines: -32000 with no data at all. A client that only reads a
// marker out of the data would miss this gate entirely.
func TestACPv2GatedFailureCanBeTheSpecificationCodeAlone(t *testing.T) {
	t.Setenv(envAuthType, authTypeAgent)
	t.Setenv(envErrorShape, errorShapeCode)
	peer, _, _ := newTestV2Peer(t)

	gated, _ := peer.prompt(promptRequest(1, "hello"))
	if gated.Error == nil || gated.Error.Code != -32000 {
		t.Fatalf("the gate must carry the specification's code, got %+v", gated.Error)
	}
	if gated.Error.Data != nil {
		t.Fatalf("the code-only shape must not carry data, got %+v", gated.Error.Data)
	}
}

// TestACPv2AdvertisesOnlyTheMethodsItsTypeAllows keeps each type honest: the
// agent type never offers a terminal method (even when the client advertised the
// capability) and the terminal type never offers a wire login.
func TestACPv2AdvertisesOnlyTheMethodsItsTypeAllows(t *testing.T) {
	cases := []struct {
		authType string
		want     []string
	}{
		{authType: authTypeAgent, want: []string{authTypeAgent}},
		{authType: authTypeTerminal, want: []string{"terminal"}},
		{authType: authTypeBoth, want: []string{authTypeAgent, "terminal"}},
	}
	for _, tc := range cases {
		t.Run(tc.authType, func(t *testing.T) {
			t.Setenv(envAuthType, tc.authType)
			peer, _, _ := newTestV2Peer(t)
			resp := peer.initialize(initializeRequest(1, `{"protocolVersion":2,"capabilities":{"auth":{"terminal":{}}},"info":{"name":"matrix"}}`))
			if resp.Error != nil {
				t.Fatalf("handshake refused: %+v", resp.Error)
			}
			var result struct {
				AuthMethods []struct {
					Type string `json:"type"`
				} `json:"authMethods"`
			}
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("decode: %v", err)
			}
			got := make([]string, 0, len(result.AuthMethods))
			for _, method := range result.AuthMethods {
				got = append(got, method.Type)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("advertised types %v, want %v", got, tc.want)
			}
		})
	}

	t.Setenv(envAuthType, authTypeTerminal)
	peer, _, _ := newTestV2Peer(t)
	resp := peer.initialize(initializeRequest(1, `{"protocolVersion":2,"capabilities":{},"info":{"name":"matrix"}}`))
	if resp.Error != nil {
		t.Fatalf("handshake refused: %+v", resp.Error)
	}
	var withoutCapability struct {
		AuthMethods []json.RawMessage `json:"authMethods"`
	}
	if err := json.Unmarshal(resp.Result, &withoutCapability); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(withoutCapability.AuthMethods) != 0 {
		t.Fatalf("a client without the capability must not be offered a terminal method: %+v", withoutCapability.AuthMethods)
	}
}

// TestACPv2RecordsTheCapabilityKeysTheClientAdvertised is the observation the
// client-capability audit needs: the peer reports which capability names it was
// allowed to act on, so a request that advertises a removed surface is visible
// instead of being inferred from the client code.
func TestACPv2RecordsTheCapabilityKeysTheClientAdvertised(t *testing.T) {
	peer, logPath, _ := newTestV2Peer(t)

	resp := peer.initialize(initializeRequest(1, `{"protocolVersion":2,"capabilities":{"auth":{"terminal":{}},"elicitation":{"form":{}}},"info":{"name":"matrix"}}`))
	if resp.Error != nil {
		t.Fatalf("handshake refused: %+v", resp.Error)
	}
	seen, ok := findPeerRecord(readPeerRecords(t, logPath), "initialize_seen")
	if !ok {
		t.Fatal("the handshake must be recorded")
	}
	keys, _ := seen.Details["capabilityKeys"].([]interface{})
	if len(keys) != 2 || keys[0] != "auth" || keys[1] != "elicitation" {
		t.Fatalf("the advertised capability keys must be recorded, got %+v", seen.Details["capabilityKeys"])
	}
}

// TestACPv2ImplementsTheSessionSurfaceItAdvertises keeps the peer honest about
// capabilities.session: the baseline methods it claims are the ones it answers.
func TestACPv2ImplementsTheSessionSurfaceItAdvertises(t *testing.T) {
	peer, logPath, _ := newTestV2Peer(t)

	for id, method := range map[int]string{1: "session/list", 2: "session/resume", 3: "session/close"} {
		resp, _ := peer.handle(jsonRPCRequest{ID: id, Method: method, Params: json.RawMessage(`{"sessionId":"mock-session-id"}`)})
		if resp.Error != nil {
			t.Fatalf("%s must be answered, got %+v", method, resp.Error)
		}
		if len(resp.Result) == 0 {
			t.Fatalf("%s must answer with a result, got none", method)
		}
	}
	records := readPeerRecords(t, logPath)
	for _, method := range []string{"session/list", "session/resume", "session/close"} {
		found := false
		for _, record := range records {
			if record.Kind == "request" && record.Method == method {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s must be recorded as received: %+v", method, records)
		}
	}
}
