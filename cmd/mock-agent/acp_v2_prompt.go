package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// ----------------------------------------------------------------------------
// What the version 2 peer does with an accepted prompt
//
// Version 2 made the prompt response an acknowledgement of insertion: the turn
// continues as session/update notifications, and the idle state update is what
// ends it. The modes below let a test drive the lifecycle the specification
// defines, and the peers a client has to survive: one that streams every variant
// and reports the terminal state late, one that says nothing after the
// acknowledgement, and the generation 1 answer that carries a stop reason and no
// idle state at all.
// ----------------------------------------------------------------------------

const (
	// promptUpsertText is what the rich turn's agent_message upsert replaces the
	// streamed chunk with, so a client that ignores upserts shows the text twice
	// and a client that applies them shows it once.
	promptUpsertText = "v2 prompt accepted (upserted)"
	// promptLateText arrives after the terminal delay, which is longer than the
	// quiet window a client that ends a turn on the prompt response would wait.
	promptLateText = " v2 late content"

	// promptModeLifecycle acknowledges, streams content and finishes with an
	// idle state update carrying the stop reason.
	promptModeLifecycle = "lifecycle"
	// promptModeRich streams every update variant the v2 lifecycle defines and
	// reports the terminal state late.
	promptModeRich = "rich"
	// promptModeSilent acknowledges insertion and never reports anything else:
	// the peer that would hold a turn open forever without a client budget.
	promptModeSilent = "silent"
	// promptModeLegacy answers the way the version 1 generation did, with a stop
	// reason on the response and no idle state update at all.
	promptModeLegacy = "legacy"

	// The identifiers below are the contract a test asserts against when it
	// follows one message, one tool call or one terminal through a whole turn.
	userMessageID  = "msg_user_mock_1"
	agentMessageID = "msg_agent_mock_1"
	lateMessageID  = "msg_agent_mock_2"
	toolCallID     = "call_mock_1"
	terminalID     = "term_mock_1"
)

// prompt is the gated operation. An unauthenticated peer answers with the
// structured auth_required error, which is what a v2 client reads to decide that
// a login has to happen before the request is retried.
func (p *acpV2Peer) prompt(req jsonRPCRequest) (jsonRPCResponse, bool) {
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID}
	var params struct {
		SessionID string       `json:"sessionId"`
		Prompt    []promptPart `json:"prompt"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		resp.Error = &jsonRPCError{Code: -32602, Message: "session/prompt params must be a JSON object"}
		return resp, true
	}
	p.record("request", "session/prompt", map[string]interface{}{
		"sessionId":     params.SessionID,
		"authenticated": p.authenticated,
		"promptMode":    p.promptMode,
	})
	if !p.authenticated {
		resp.Error = p.authenticationRequiredError()
		return resp, true
	}
	if p.promptMode == promptModeSilent || p.promptMode == promptModeLegacy {
		if p.promptMode == promptModeLegacy {
			writeMessageNotification(params.SessionID, promptAcceptedText)
		}
		resp.Result = promptAcknowledgement(p.promptMode == promptModeLegacy)
		return resp, true
	}
	// The acknowledgement goes out before any of the turn's work does, because
	// that is what insertion means in version 2: the response reports that the
	// user message entered the conversation, and everything else — content,
	// progress and completion — arrives as session/update notifications. The
	// request is answered here rather than by the caller so the ordering is the
	// one the specification describes.
	writeJSON(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: promptAcknowledgement(false)})
	if p.promptMode == promptModeRich {
		p.streamRichTurn(params.SessionID)
	} else {
		p.streamLifecycleTurn(params.SessionID)
	}
	return jsonRPCResponse{}, false
}

// promptAcknowledgement is the successful prompt response. Version 2 requires
// the inserted user message's identifier and defines no stop reason on the
// response at all; the legacy answer keeps the generation 1 shape so a client
// that only understands that generation stays testable.
func promptAcknowledgement(legacy bool) json.RawMessage {
	if legacy {
		return json.RawMessage(`{"stopReason": "end_turn"}`)
	}
	return json.RawMessage(`{"messageId": "` + userMessageID + `"}`)
}

// streamLifecycleTurn is the canonical version 2 turn: the user message is
// echoed, foreground work reports running, the answer streams, and an idle state
// update with the stop reason ends it.
func (p *acpV2Peer) streamLifecycleTurn(sessionID string) {
	writeSessionUpdate(sessionID, map[string]interface{}{
		"sessionUpdate": "user_message",
		"messageId":     userMessageID,
		"content":       []map[string]interface{}{{"type": "text", "text": promptAcceptedText}},
	})
	writeSessionUpdate(sessionID, map[string]interface{}{"sessionUpdate": "state_update", "state": "running"})
	writeAgentChunk(sessionID, agentMessageID, promptAcceptedText)
	writeSessionUpdate(sessionID, map[string]interface{}{
		"sessionUpdate": "state_update", "state": "idle", "stopReason": "end_turn",
	})
}

// streamRichTurn drives every session update a version 2 turn can carry, and
// reports the terminal state after the configured delay so a caller can prove a
// turn waits for the terminal signal instead of for a quiet moment.
func (p *acpV2Peer) streamRichTurn(sessionID string) {
	writeSessionUpdate(sessionID, map[string]interface{}{
		"sessionUpdate": "user_message",
		"messageId":     userMessageID,
		"content":       []map[string]interface{}{{"type": "text", "text": promptAcceptedText}},
	})
	writeSessionUpdate(sessionID, map[string]interface{}{"sessionUpdate": "state_update", "state": "running"})
	writeSessionUpdate(sessionID, map[string]interface{}{
		"sessionUpdate": "plan_update",
		"plan": map[string]interface{}{
			"type": "items", "planId": "plan-mock-1",
			"entries": []map[string]interface{}{
				{"content": "read the file", "priority": "high", "status": "in_progress"},
				{"content": "report the result", "priority": "medium", "status": "pending"},
			},
		},
	})
	writeAgentChunk(sessionID, agentMessageID, promptAcceptedText)
	writeSessionUpdate(sessionID, map[string]interface{}{
		"sessionUpdate": "tool_call_update", "toolCallId": toolCallID,
		"title": "read the file", "kind": "read", "status": "in_progress",
	})
	p.streamRichToolUpdates(sessionID)
	if p.terminalDelay > 0 {
		time.Sleep(p.terminalDelay)
	}
	writeAgentChunk(sessionID, lateMessageID, promptLateText)
	writeSessionUpdate(sessionID, map[string]interface{}{
		"sessionUpdate": "state_update", "state": "idle", "stopReason": "end_turn",
	})
}

// streamRichToolUpdates is the tool and terminal half of the rich turn: a diff,
// an agent-owned terminal and its output, all in the shapes version 2 defined.
func (p *acpV2Peer) streamRichToolUpdates(sessionID string) {
	writeSessionUpdate(sessionID, map[string]interface{}{
		"sessionUpdate": "tool_call_content_chunk", "toolCallId": toolCallID,
		"content": map[string]interface{}{
			"type":    "content",
			"content": map[string]interface{}{"type": "text", "text": "read 12 lines"},
		},
	})
	writeSessionUpdate(sessionID, map[string]interface{}{
		"sessionUpdate": "tool_call_update", "toolCallId": toolCallID, "status": "completed",
		"content": []map[string]interface{}{{
			"type": "diff",
			"changes": []map[string]interface{}{
				{"operation": "modify", "path": "/tmp/mock-agent/main.go", "fileType": "text"},
			},
			"patch": map[string]interface{}{
				"format": "git_patch",
				"text":   "--- a/main.go\n+++ b/main.go\n",
			},
		}},
	})
	writeSessionUpdate(sessionID, map[string]interface{}{
		"sessionUpdate": "terminal_update", "terminalId": terminalID,
		"command": "go test ./...", "cwd": "/tmp/mock-agent",
		"output": map[string]interface{}{"data": base64.StdEncoding.EncodeToString([]byte("ok\n"))},
	})
	writeSessionUpdate(sessionID, map[string]interface{}{
		"sessionUpdate": "terminal_output_chunk", "terminalId": terminalID,
		"data": base64.StdEncoding.EncodeToString([]byte("more output\n")),
	})
	writeSessionUpdate(sessionID, map[string]interface{}{
		"sessionUpdate": "agent_message", "messageId": agentMessageID,
		"content": []map[string]interface{}{{"type": "text", "text": promptUpsertText}},
	})
}

// writeSessionUpdate sends one session/update notification.
func writeSessionUpdate(sessionID string, update map[string]interface{}) {
	params, err := json.Marshal(map[string]interface{}{"sessionId": sessionID, "update": update})
	if err != nil {
		return
	}
	writeJSON(jsonRPCResponse{JSONRPC: "2.0", Method: ptr("session/update"), Params: json.RawMessage(params)})
}

// writeAgentChunk sends one streamed agent message chunk, which version 2
// requires to carry the message identifier its chunks belong to.
func writeAgentChunk(sessionID, messageID, text string) {
	writeSessionUpdate(sessionID, map[string]interface{}{
		"sessionUpdate": "agent_message_chunk", "messageId": messageID,
		"content": map[string]interface{}{"type": "text", "text": text},
	})
}

// promptModeFromEnv reads the configured prompt behaviour, defaulting to the
// version 2 lifecycle. An unknown value is reported rather than silently
// treated as the default, because a test that misspells a mode would otherwise
// assert against the wrong peer.
func promptModeFromEnv() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(envPromptMode)))
	switch mode {
	case "", promptModeLifecycle, promptModeRich, promptModeSilent, promptModeLegacy:
		if mode == "" {
			return promptModeLifecycle
		}
		return mode
	default:
		fmt.Fprintln(os.Stderr, "mock-agent acp v2: unknown prompt mode, using lifecycle:", mode)
		return promptModeLifecycle
	}
}

// durationFromEnv reads a Go duration, treating an absent or invalid value as
// zero delay.
func durationFromEnv(name string) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name)))
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}
