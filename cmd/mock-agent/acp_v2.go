package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ----------------------------------------------------------------------------
// ACP v2 peer mode
//
// The default mock is a version 1 peer: that is what the elicitation and
// terminal interop tests drive. This file turns the same binary into a version 2
// peer when it is started with --acp-v2, because the v2 authentication contract
// needs a peer that really refuses what v2 forbids rather than a fake that
// agrees with whatever the client sent:
//
//   - initialize negotiates version 2 only and requires the parameter names v2
//     defined, capabilities and info. A request still carrying v1's
//     clientCapabilities/clientInfo is answered with a JSON-RPC error, so a
//     client that keeps the old wire names cannot complete the handshake at all.
//   - the handshake advertises one authentication method of type "terminal",
//     carrying the args and env that tell the client how to run the login
//     program itself, and only when the client advertised
//     capabilities.auth.terminal, which is the condition v2 puts on offering
//     one. ACP v2 forbids auth/login for that type, so the peer answers
//     auth/login with an error instead of accepting it.
//   - session/prompt is gated behind that login: until the credential file the
//     login program writes exists, a prompt fails with the structured
//     auth_required marker v2 defines for exactly this case. The gate is read
//     once, at process startup, so a login that runs without the connection
//     being rebuilt leaves the peer unauthenticated.
//
// Every observation is appended as one JSON object per line to the file named by
// MOCK_AGENT_LOG_PATH and echoed to stderr, so a test can assert what actually
// crossed the wire: the initialize parameter names, every method name the peer
// was sent, and which process each one arrived on. The login program leaves its
// own marker, the credential file, whose content is what that process saw.
// ----------------------------------------------------------------------------

const (
	// acpV2Flag switches the peer to the version 2 behaviour below.
	acpV2Flag = "--acp-v2"
	// terminalLoginFlag is the login program's second mode: the terminal
	// authentication method advertises it as an extra argument.
	terminalLoginFlag = "--terminal-login"
	// terminalMethodID is the identifier the handshake advertises.
	terminalMethodID = "terminal-login"

	// envCredentialPath is where the login program writes what it saw and where
	// the peer reads whether a login has happened.
	envCredentialPath = "MOCK_AGENT_CREDENTIAL_PATH"
	// envMethodToken names the variable the advertised method contributes, and
	// the login program reports its value.
	envMethodToken = "MOCK_AGENT_V2_METHOD_TOKEN"
	// envBaseToken is the same-named variable the base launch configuration
	// carries, which the method's env must override.
	envBaseToken = "MOCK_AGENT_TERMINAL_TOKEN"
	// envPeerLogPath names the file every observation is appended to.
	envPeerLogPath = "MOCK_AGENT_LOG_PATH"

	// promptAcceptedText is what an authenticated prompt answers, so a caller can
	// tell the retried request apart from the gated one.
	promptAcceptedText = "v2 prompt accepted"
)

// jsonRPCError is the error half of a JSON-RPC 2.0 response. The v1 paths never
// needed one; the v2 peer uses it for the refused handshake and the gated
// prompt, which the client must see as failures rather than as empty results.
type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// acpV2Peer is the per-process state of the version 2 peer. Authentication is
// resolved once at startup from the credential file, which is what makes a
// reconnect observable: only a new process can see a credential written after
// the old one started.
type acpV2Peer struct {
	credentialPath string
	logPath        string
	authenticated  bool
}

// newACPV2PeerFromArgs builds the peer for a --acp-v2 process and returns nil
// for the default version 1 peer, whose behaviour stays exactly what it was.
func newACPV2PeerFromArgs(args []string) *acpV2Peer {
	if !hasArg(args, acpV2Flag) {
		return nil
	}
	peer := &acpV2Peer{
		credentialPath: strings.TrimSpace(os.Getenv(envCredentialPath)),
		logPath:        strings.TrimSpace(os.Getenv(envPeerLogPath)),
	}
	peer.authenticated = fileExists(peer.credentialPath)
	peer.record("startup", "", map[string]interface{}{
		"authenticated": peer.authenticated,
		"credential":    peer.credentialPath,
	})
	return peer
}

// handle answers one request as a version 2 peer. Every method name is recorded
// before it is answered, because that recording is the evidence a test asserts
// against: a terminal login must never show up there as auth/login.
func (p *acpV2Peer) handle(req jsonRPCRequest) jsonRPCResponse {
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		return p.initialize(req)
	case "session/prompt":
		return p.prompt(req)
	case "session/new":
		p.record("request", req.Method, nil)
		resp.Result = json.RawMessage(`{"sessionId": "mock-session-id"}`)
	case "auth/login", "authenticate":
		p.record("request", req.Method, nil)
		resp.Error = &jsonRPCError{
			Code:    -32601,
			Message: "a terminal authentication method is completed by running the agent program, not by " + req.Method,
		}
	default:
		p.record("request", req.Method, nil)
	}
	return resp
}

// initialize accepts exactly what version 2 defines. The parameter names that
// arrived are recorded either way, and the v1 names are a hard failure: an agent
// implementing the specification reads capabilities and info, so a request
// carrying the older names advertises nothing and must not be treated as a
// successful handshake.
func (p *acpV2Peer) initialize(req jsonRPCRequest) jsonRPCResponse {
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID}
	observed, rejection := decodeV2InitializeParams(req.Params)
	observed["accepted"] = rejection == ""
	p.record("initialize_seen", "initialize", observed)
	if rejection == "" {
		resp.Result = p.initializeResult(observed["authTerminalCapability"] == true)
		return resp
	}
	p.record("initialize_rejected", "initialize", map[string]interface{}{"reason": rejection})
	resp.Error = &jsonRPCError{Code: -32602, Message: rejection}
	return resp
}

// decodeV2InitializeParams reports what the request carried and whether it is
// acceptable. The observation is returned even for a rejected request, because
// the names are the wire contract under test.
func decodeV2InitializeParams(raw json.RawMessage) (map[string]interface{}, string) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return map[string]interface{}{"paramNames": []string{}}, "initialize params must be a JSON object"
	}
	version := decodeIntField(fields, "protocolVersion")
	capabilities := fields["capabilities"] != nil
	info := fields["info"] != nil
	v1Capabilities := fields["clientCapabilities"] != nil
	v1Info := fields["clientInfo"] != nil
	authTerminal := terminalAuthAdvertised(fields)
	observed := map[string]interface{}{
		"protocolVersion":        version,
		"paramNames":             sortedFieldNames(fields),
		"hasCapabilities":        capabilities,
		"hasInfo":                info,
		"hasV1Capabilities":      v1Capabilities,
		"hasV1Info":              v1Info,
		"authTerminalCapability": authTerminal,
	}
	switch {
	case version < 2:
		return observed, "unsupported protocol version: this peer speaks ACP version 2 only"
	case v1Capabilities || v1Info:
		return observed, "invalid params for protocol version 2: capabilities and info supersede clientCapabilities and clientInfo"
	case !capabilities || !info:
		return observed, "invalid params for protocol version 2: capabilities and info are required"
	}
	return observed, ""
}

// terminalAuthAdvertised reads capabilities.auth.terminal. Version 2 defines the
// capability as the presence of a non-null object, and an agent may advertise a
// terminal method only when the client sent it, so this decides whether the
// handshake answer carries one at all.
func terminalAuthAdvertised(fields map[string]json.RawMessage) bool {
	var capabilities struct {
		Auth struct {
			Terminal json.RawMessage `json:"terminal"`
		} `json:"auth"`
	}
	if err := json.Unmarshal(fields["capabilities"], &capabilities); err != nil {
		return false
	}
	terminal := strings.TrimSpace(string(capabilities.Auth.Terminal))
	return terminal != "" && terminal != "null"
}

// initializeResult advertises version 2 and, when the client advertised that it
// can reproduce the agent invocation, one terminal method. Version 2 renamed the
// response's capability and implementation fields to capabilities and info, so an
// answer carrying v1's agentCapabilities/agentInfo would not be answering this
// generation. The method's args are appended to this peer's own launch arguments
// and its env overrides a same-named variable of the base launch configuration,
// so a client that really runs the method reproduces the invocation asked for.
func (p *acpV2Peer) initializeResult(terminalCapable bool) json.RawMessage {
	result := map[string]interface{}{
		"protocolVersion": 2,
		"capabilities":    map[string]interface{}{},
		"info": map[string]interface{}{
			"name":    "mock-agent",
			"title":   "Matrix mock ACP agent",
			"version": "0.0.0-test",
		},
	}
	if terminalCapable {
		result["authMethods"] = []interface{}{p.terminalMethod()}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return json.RawMessage(`{"protocolVersion": 2, "capabilities": {}, "info": {"name": "mock-agent"}}`)
	}
	return encoded
}

// terminalMethod is the advertised terminal login. The env entry names the same
// variable the base launch configuration carries on purpose: overriding it is
// what the client has to reproduce, and the login program reports the value it
// actually saw.
func (p *acpV2Peer) terminalMethod() map[string]interface{} {
	return map[string]interface{}{
		"type":        "terminal",
		"methodId":    terminalMethodID,
		"name":        "Terminal login",
		"description": "Runs the agent program with " + terminalLoginFlag + " to leave a credential.",
		"args":        []string{terminalLoginFlag},
		"env": []map[string]string{{
			"name":  envBaseToken,
			"value": os.Getenv(envMethodToken),
		}},
	}
}

// prompt is the gated operation. An unauthenticated peer answers with the
// structured auth_required marker in the error data, which is what a v2 client
// reads to decide that a login has to happen before the request is retried.
func (p *acpV2Peer) prompt(req jsonRPCRequest) jsonRPCResponse {
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID}
	var params struct {
		SessionID string       `json:"sessionId"`
		Prompt    []promptPart `json:"prompt"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		resp.Error = &jsonRPCError{Code: -32602, Message: "session/prompt params must be a JSON object"}
		return resp
	}
	p.record("request", "session/prompt", map[string]interface{}{
		"sessionId":     params.SessionID,
		"authenticated": p.authenticated,
	})
	if !p.authenticated {
		resp.Error = &jsonRPCError{
			Code:    -32000,
			Message: "authentication required",
			Data: map[string]interface{}{
				"kind":     "auth_required",
				"methodId": terminalMethodID,
				"message":  "run the terminal authentication method before prompting",
			},
		}
		return resp
	}
	writeMessageNotification(params.SessionID, promptAcceptedText)
	resp.Result = json.RawMessage(`{"stopReason": "end_turn"}`)
	return resp
}

// record appends one observation to the peer log and echoes it to stderr.
func (p *acpV2Peer) record(kind, method string, details map[string]interface{}) {
	record := peerLogRecord{Kind: kind, Method: method, PID: os.Getpid(), Details: details}
	encoded, err := json.Marshal(record)
	if err == nil {
		fmt.Fprintln(os.Stderr, "mock-agent acp v2:", string(encoded))
	}
	appendPeerLog(p.logPath, record)
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func sortedFieldNames(fields map[string]json.RawMessage) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func decodeIntField(fields map[string]json.RawMessage, name string) int {
	var value int
	if err := json.Unmarshal(fields[name], &value); err != nil {
		return 0
	}
	return value
}
