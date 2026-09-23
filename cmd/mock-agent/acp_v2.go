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
// peer when it is started with --acp-v2, because the v2 contract needs a peer
// that really refuses what v2 forbids rather than a fake that agrees with
// whatever the client sent:
//
//   - initialize negotiates version 2 only and requires the parameter names v2
//     defined, capabilities and info. A request still carrying v1's
//     clientCapabilities/clientInfo is answered with a JSON-RPC error, so a
//     client that keeps the old wire names cannot complete the handshake at all.
//     The names, and the capability keys, that arrived are recorded, because
//     what a peer may call is decided by what the client actually advertised.
//   - the handshake advertises the authentication methods
//     MOCK_AGENT_V2_AUTH_TYPE selects. The methods themselves, the
//     protocol-driven login, the logout and the gated failure live in
//     acp_v2_auth.go.
//   - session/prompt is gated behind the login: until the peer is authenticated
//     a prompt fails with the structured auth_required error v2 defines for
//     exactly this case. The terminal method's credential file is read once, at
//     process startup, so a terminal login that runs without the connection
//     being rebuilt leaves the peer unauthenticated; an agent login takes effect
//     on the running process, as the protocol-driven flow requires.
//     MOCK_AGENT_V2_ERROR_SHAPE=code sends the failure with no data at all, so a
//     test can prove the specification's error code alone is understood.
//   - the session surface the handshake advertises is the one the peer really
//     answers: session/new, session/list, session/resume, session/close and
//     session/prompt.
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
	// terminalMethodID is the identifier of the terminal method.
	terminalMethodID = "terminal-login"
	// agentMethodID is the identifier of the method the agent handles itself
	// through auth/login.
	agentMethodID = "agent-login"

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
	// envAuthType selects which authentication methods the peer advertises.
	envAuthType = "MOCK_AGENT_V2_AUTH_TYPE"
	// envErrorShape selects how the gated failure is reported.
	envErrorShape = "MOCK_AGENT_V2_ERROR_SHAPE"
	// envForceTerminal makes the peer offer its terminal method even when the
	// client did not advertise the capability. The specification forbids that,
	// which is the point: it models the peer the client has to refuse when the
	// operator has not opted in.
	envForceTerminal = "MOCK_AGENT_V2_FORCE_TERMINAL"

	// authTypeAgent advertises only the agent-handled method, authTypeTerminal
	// only the terminal one (the default, so the existing terminal interop test
	// keeps driving the same peer), and authTypeBoth advertises both.
	authTypeAgent    = "agent"
	authTypeTerminal = "terminal"
	authTypeBoth     = "both"
	// errorShapeCode reports the gated failure with the specification's code and
	// no data; every other value also carries the auth_required marker.
	errorShapeCode = "code"

	// promptAcceptedText is what an authenticated prompt answers, so a caller can
	// tell the retried request apart from the gated one.
	promptAcceptedText = "v2 prompt accepted"
)

// jsonRPCError is the error half of a JSON-RPC 2.0 response. The v1 paths never
// needed one; the v2 peer uses it for the refused handshake, the forbidden wire
// login and the gated prompt, which the client must see as failures rather than
// as empty results.
type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// acpV2Peer is the per-process state of the version 2 peer. A terminal login is
// resolved once at startup from the credential file, which is what makes a
// reconnect observable: only a new process can see a credential written after
// the old one started. An agent login is protocol-driven, so it changes this
// state in place, and auth/logout clears it again.
type acpV2Peer struct {
	credentialPath string
	logPath        string
	authType       string
	errorShape     string
	authenticated  bool
	// forceTerminal makes the peer offer a terminal method the client did not
	// advertise the capability for, which is how the client's refusal of that
	// method is exercised.
	forceTerminal bool
	// clientTerminalCapable records what the client advertised, because an agent
	// may offer a terminal method only when the capability is present.
	clientTerminalCapable bool
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
		authType:       authTypeFromEnv(),
		errorShape:     strings.ToLower(strings.TrimSpace(os.Getenv(envErrorShape))),
		forceTerminal:  strings.EqualFold(strings.TrimSpace(os.Getenv(envForceTerminal)), "true"),
	}
	peer.authenticated = fileExists(peer.credentialPath)
	peer.record("startup", "", map[string]interface{}{
		"authenticated": peer.authenticated,
		"credential":    peer.credentialPath,
		"authType":      peer.authType,
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
	case "session/list":
		p.record("request", req.Method, nil)
		resp.Result = json.RawMessage(`{"sessions": [{"sessionId": "mock-session-id", "title": "mock session"}]}`)
	case "session/resume", "session/close":
		p.record("request", req.Method, nil)
		resp.Result = json.RawMessage(`{}`)
	case "auth/login", "authenticate":
		return p.login(req)
	case "auth/logout":
		return p.logout(req)
	default:
		p.record("request", req.Method, nil)
		resp.Error = &jsonRPCError{Code: -32601, Message: "this peer does not implement " + req.Method}
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
		p.clientTerminalCapable = observed["authTerminalCapability"] == true
		resp.Result = p.initializeResult()
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
		"capabilityKeys":         sortedCapabilityKeys(fields["capabilities"]),
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

// initializeResult advertises version 2, the session surface this peer really
// implements, and the configured authentication methods. Version 2 renamed the
// response's capability and implementation fields to capabilities and info, so an
// answer carrying v1's agentCapabilities/agentInfo would not be answering this
// generation.
func (p *acpV2Peer) initializeResult() json.RawMessage {
	result := map[string]interface{}{
		"protocolVersion": 2,
		"capabilities": map[string]interface{}{
			// The baseline session methods v2 makes implicit in this object:
			// session/new, session/list, session/resume, session/close,
			// session/prompt, session/cancel and session/update.
			"session": map[string]interface{}{},
		},
		"info": map[string]interface{}{
			"name":    "mock-agent",
			"title":   "Matrix mock ACP agent",
			"version": "0.0.0-test",
		},
	}
	if methods := p.advertisedMethods(); len(methods) > 0 {
		result["authMethods"] = methods
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return json.RawMessage(`{"protocolVersion": 2, "capabilities": {}, "info": {"name": "mock-agent"}}`)
	}
	return encoded
}

// prompt is the gated operation. An unauthenticated peer answers with the
// structured auth_required error, which is what a v2 client reads to decide that
// a login has to happen before the request is retried.
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
		resp.Error = p.authenticationRequiredError()
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

// sortedCapabilityKeys reports the top-level capability names a client
// advertised, so a test can assert what the peer was actually allowed to call.
func sortedCapabilityKeys(raw json.RawMessage) []string {
	var capabilities map[string]json.RawMessage
	if err := json.Unmarshal(raw, &capabilities); err != nil {
		return []string{}
	}
	return sortedFieldNames(capabilities)
}

func decodeIntField(fields map[string]json.RawMessage, name string) int {
	var value int
	if err := json.Unmarshal(fields[name], &value); err != nil {
		return 0
	}
	return value
}
