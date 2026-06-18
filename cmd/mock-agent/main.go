// Package main implements a mock ACP agent for testing.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Method  *string         `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type promptPart struct {
	Text string `json:"text"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req jsonRPCRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		resp, ok := handleRequest(req, scanner)
		if !ok {
			continue
		}
		writeJSON(resp)
	}
}

func handleRequest(req jsonRPCRequest, scanner *bufio.Scanner) (jsonRPCResponse, bool) {
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = json.RawMessage(`{"protocolVersion": 1, "agentCapabilities": {}}`)
	case "session/new":
		resp.Result = json.RawMessage(`{"sessionId": "mock-session-id"}`)
	case "session/prompt":
		result, ok := handlePrompt(req, scanner)
		if !ok {
			return jsonRPCResponse{}, false
		}
		resp.Result = result
	}
	return resp, true
}

func handlePrompt(req jsonRPCRequest, scanner *bufio.Scanner) (json.RawMessage, bool) {
	var params struct {
		SessionID string       `json:"sessionId"`
		Prompt    []promptPart `json:"prompt"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, false
	}
	if promptHasText(params.Prompt, "__TERMINAL_TEST__") {
		writeTerminalRequest()
		writeTerminalNotification(scanner, params.SessionID)
	} else {
		writeMessageNotification(params.SessionID, "I am a mock agent responding via stdio.")
	}
	return json.RawMessage(`{"stopReason": "end_turn"}`), true
}

func promptHasText(prompt []promptPart, text string) bool {
	for _, part := range prompt {
		if part.Text == text {
			return true
		}
	}
	return false
}

func writeTerminalRequest() {
	callClient(nil, 100, "terminal/create", map[string]interface{}{"command": "echo", "args": []string{"from-mock-agent"}})
}

func writeTerminalNotification(scanner *bufio.Scanner, sessionID string) {
	createResult, ok := readClientResponse(scanner)
	if !ok {
		return
	}
	var create struct {
		TerminalID string `json:"terminalId"`
	}
	if json.Unmarshal(createResult, &create) != nil || create.TerminalID == "" {
		writeMessageNotification(sessionID, "terminal result: "+string(createResult))
		return
	}
	waitResult, ok := callClient(scanner, 101, "terminal/wait_for_exit", map[string]interface{}{"terminalId": create.TerminalID})
	if !ok {
		return
	}
	outputResult, ok := callClient(scanner, 102, "terminal/output", map[string]interface{}{"terminalId": create.TerminalID})
	if !ok {
		return
	}
	_, _ = callClient(scanner, 103, "terminal/release", map[string]interface{}{"terminalId": create.TerminalID})
	writeMessageNotification(sessionID, "terminal result: "+string(waitResult)+" output: "+string(outputResult))
}

func callClient(scanner *bufio.Scanner, id int, method string, params map[string]interface{}) (json.RawMessage, bool) {
	paramsBytes, _ := json.Marshal(params)
	writeJSON(jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  json.RawMessage(paramsBytes),
	})
	if scanner == nil {
		return nil, true
	}
	return readClientResponse(scanner)
}

func readClientResponse(scanner *bufio.Scanner) (json.RawMessage, bool) {
	if !scanner.Scan() {
		return nil, false
	}
	var resp jsonRPCResponse
	if json.Unmarshal(scanner.Bytes(), &resp) != nil || resp.Result == nil {
		return nil, false
	}
	return resp.Result, true
}

func writeMessageNotification(sessionID, text string) {
	params := map[string]interface{}{
		"sessionId": sessionID,
		"update": map[string]interface{}{
			"sessionUpdate": "agent_message_chunk",
			"content":       map[string]interface{}{"type": "text", "text": text},
		},
	}
	paramBytes, _ := json.Marshal(params)
	writeJSON(jsonRPCResponse{
		JSONRPC: "2.0",
		Method:  ptr("session/update"),
		Params:  json.RawMessage(paramBytes),
	})
}

func writeJSON(value interface{}) {
	payload, err := json.Marshal(value)
	if err == nil {
		fmt.Println(string(payload))
	}
}

func ptr(s string) *string {
	return &s
}
