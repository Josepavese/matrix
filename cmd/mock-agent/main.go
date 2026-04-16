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

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
		}

		switch req.Method {
		case "initialize":
			resp.Result = json.RawMessage(`{"capabilities": {"edit": {}}}`)
		case "session/new":
			var params struct { // Renamed from 'req' to 'params' to avoid conflict with outer 'req'
				ClientTitle string `json:"clientTitle"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				continue
			}

			// The original instruction had 'middleware.NewSessionResponse' and 'writeResp'.
			// These are not defined in the provided context.
			// I'm adapting to the existing structure by setting resp.Result directly.
			// If 'middleware' and 'writeResp' are meant to be added, they need to be defined.
			resp.Result = json.RawMessage(`{"sessionId": "mock-session-id"}`)

		case "session/prompt":
			var params struct {
				SessionID string `json:"sessionId"`
				Prompt    []struct {
					Text string `json:"text"`
				} `json:"prompt"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				continue
			}

			// Check if the prompt requests a terminal/create test
			terminalTest := false
			for _, p := range params.Prompt {
				if p.Text == "__TERMINAL_TEST__" {
					terminalTest = true
					break
				}
			}

			if terminalTest {
				// Send a terminal/create request to the client
				termReq := jsonRPCRequest{
					JSONRPC: "2.0",
					ID:      100,
					Method:  "terminal/create",
					Params:  json.RawMessage(`{"command":"echo","args":["from-mock-agent"]}`),
				}
				tBytes, _ := json.Marshal(termReq)
				fmt.Println(string(tBytes))

				// Read the response from the client
				if scanner.Scan() {
					var termResp jsonRPCResponse
					if json.Unmarshal(scanner.Bytes(), &termResp) == nil && termResp.Result != nil {
						// Build the notification with properly JSON-escaped result
						resultText := string(termResp.Result)
						notifParams := map[string]interface{}{
							"sessionId": params.SessionID,
							"update": map[string]interface{}{
								"sessionUpdate": "agent_message_chunk",
								"content": map[string]interface{}{
									"type": "text",
									"text": "terminal result: " + resultText,
								},
							},
						}
						notifParamsBytes, _ := json.Marshal(notifParams)
						notif := jsonRPCResponse{
							JSONRPC: "2.0",
							Method:  ptr("session/update"),
							Params:  json.RawMessage(notifParamsBytes),
						}
						nBytes, _ := json.Marshal(notif)
						fmt.Println(string(nBytes))
					}
				}
			} else {
				// Notify with the new session/update structured format
				notif := jsonRPCResponse{
					JSONRPC: "2.0",
					Method:  ptr("session/update"),
					Params:  json.RawMessage(fmt.Sprintf(`{"sessionId": "%s", "update": {"sessionUpdate": "agent_message_chunk", "content": {"type": "text", "text": "I am a mock agent responding via stdio."}}}`, params.SessionID)),
				}
				nBytes, err := json.Marshal(notif)
				if err != nil {
					continue
				}
				fmt.Println(string(nBytes))
			}

			resp.Result = json.RawMessage(`{"stopReason": "end_turn"}`)
		}

		rBytes, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		fmt.Println(string(rBytes))
	}
}

func ptr(s string) *string {
	return &s
}
