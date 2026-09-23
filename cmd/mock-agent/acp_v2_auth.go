package main

import (
	"encoding/json"
	"os"
	"strings"
)

// ----------------------------------------------------------------------------
// ACP v2 authentication methods for the mock peer
//
// The advertised methods, the protocol-driven login, the logout that puts the
// gate back and the gated failure itself live here; the peer, the handshake and
// the constants they share live in acp_v2.go. They are separated because the
// peer file has a size budget and because the authentication surface is the part
// another client implementation is most likely to compare against.
// ----------------------------------------------------------------------------

// authTypeFromEnv reads the advertised method selection, defaulting to the
// terminal method every existing v2 test drives.
func authTypeFromEnv() string {
	switch value := strings.ToLower(strings.TrimSpace(os.Getenv(envAuthType))); value {
	case authTypeAgent, authTypeBoth, authTypeTerminal:
		return value
	default:
		return authTypeTerminal
	}
}

// offersAgentMethod reports whether this peer advertises the agent-handled login.
func (p *acpV2Peer) offersAgentMethod() bool {
	return p.authType == authTypeAgent || p.authType == authTypeBoth
}

// advertisedMethods is what this peer offered the connected client: the agent
// method when the configured type includes it, and the terminal method only when
// the client advertised that it can reproduce the agent invocation.
func (p *acpV2Peer) advertisedMethods() []interface{} {
	methods := []interface{}{}
	if p.offersAgentMethod() {
		methods = append(methods, map[string]interface{}{
			"type":        "agent",
			"methodId":    agentMethodID,
			"name":        "Agent login",
			"description": "Logs in over the protocol with auth/login.",
		})
	}
	if p.authType != authTypeAgent && (p.clientTerminalCapable || p.forceTerminal) {
		methods = append(methods, p.terminalMethod())
	}
	return methods
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

// login completes an agent-handled method. Only a method this peer advertised
// may be used, and the terminal type never reaches this method: v2 completes it
// by running the program instead.
func (p *acpV2Peer) login(req jsonRPCRequest) jsonRPCResponse {
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID}
	var params struct {
		MethodID string `json:"methodId"`
	}
	_ = json.Unmarshal(req.Params, &params)
	p.record("request", req.Method, map[string]interface{}{"methodId": params.MethodID})
	switch {
	case params.MethodID == terminalMethodID:
		resp.Error = &jsonRPCError{
			Code:    -32601,
			Message: "a terminal authentication method is completed by running the agent program, not by " + req.Method,
		}
	case params.MethodID == agentMethodID && p.offersAgentMethod():
		p.authenticated = true
		resp.Result = json.RawMessage(`{}`)
	default:
		resp.Error = &jsonRPCError{Code: -32602, Message: "not an advertised authentication method: " + params.MethodID}
	}
	return resp
}

// logout clears the authenticated state so the gate comes back, and is refused
// when no method is advertised, because v2 forbids calling it then.
func (p *acpV2Peer) logout(req jsonRPCRequest) jsonRPCResponse {
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID}
	p.record("request", "auth/logout", nil)
	if len(p.advertisedMethods()) == 0 {
		resp.Error = &jsonRPCError{Code: -32601, Message: "no authentication method is advertised, so auth/logout is not available"}
		return resp
	}
	p.authenticated = false
	resp.Result = json.RawMessage(`{}`)
	return resp
}

// authenticationRequiredError is the gate. Version 2 assigns it the code -32000,
// "authentication is required before this operation can be performed", and this
// peer also carries the richer marker in the data unless the error shape is set
// to the code alone.
func (p *acpV2Peer) authenticationRequiredError() *jsonRPCError {
	failure := &jsonRPCError{Code: -32000, Message: "authentication required"}
	if p.errorShape == errorShapeCode {
		return failure
	}
	failure.Data = map[string]interface{}{
		"kind":     "auth_required",
		"methodId": p.gateMethodID(),
		"message":  "authenticate before prompting",
	}
	return failure
}

// gateMethodID names the method this peer expects the client to run for the
// gate, preferring the agent-handled one because it needs no user.
func (p *acpV2Peer) gateMethodID() string {
	if p.offersAgentMethod() {
		return agentMethodID
	}
	return terminalMethodID
}
