package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// ----------------------------------------------------------------------------
// The version 2 prompt lifecycle the mock peer offers
//
// The integration tests drive these modes through a real subprocess. These tests
// pin the peer's own contract, because the order of what it writes is the point:
// the acknowledgement of insertion goes out before any of the turn's work, and
// the turn ends with an idle state update. A peer that streamed first would let a
// client pass a lifecycle test by accident.
// ----------------------------------------------------------------------------

// authenticatedTestV2Peer builds a peer that is already logged in, so the prompt
// modes can be exercised without the gated authentication flow.
func authenticatedTestV2Peer(t *testing.T, mode string) *acpV2Peer {
	t.Helper()
	dir := t.TempDir()
	credentialPath := dir + "/credential.json"
	if err := os.WriteFile(credentialPath, []byte(`{"token":"test"}`), 0o600); err != nil {
		t.Fatalf("write credential: %v", err)
	}
	t.Setenv(envPeerLogPath, dir+"/methods.jsonl")
	t.Setenv(envCredentialPath, credentialPath)
	t.Setenv(envPromptMode, mode)
	peer := newACPV2PeerFromArgs([]string{acpV2Flag})
	if peer == nil || !peer.authenticated {
		t.Fatalf("the test peer must start authenticated, got %#v", peer)
	}
	return peer
}

// decodeWireFrames reports the frames a peer wrote in order, with the method of
// each notification and the update it carried.
func decodeWireFrames(t *testing.T, raw string) []map[string]interface{} {
	t.Helper()
	frames := []map[string]interface{}{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var frame map[string]interface{}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("malformed frame %q: %v", line, err)
		}
		frames = append(frames, frame)
	}
	return frames
}

func TestACPv2PromptAcknowledgesInsertionBeforeStreaming(t *testing.T) {
	peer := authenticatedTestV2Peer(t, promptModeLifecycle)
	var resp jsonRPCResponse
	answered := true
	raw := captureStdout(t, func() {
		resp, answered = peer.prompt(promptRequest(2, "hello"))
	})

	frames := decodeWireFrames(t, raw)
	if answered {
		t.Fatalf("the lifecycle turn must answer through the wire so the acknowledgement precedes its work, got %+v", resp)
	}
	if len(frames) < 2 {
		t.Fatalf("the turn must write its acknowledgement and then its work, got %d frames: %s", len(frames), raw)
	}
	ack, _ := frames[0]["result"].(map[string]interface{})
	if ack["messageId"] != userMessageID {
		t.Fatalf("the acknowledgement must carry the inserted message id v2 requires, got %#v", frames[0]["result"])
	}
	if _, ok := ack["stopReason"]; ok {
		t.Fatalf("version 2 defines no stop reason on the response, got %#v", ack)
	}
	if frames[1]["method"] != "session/update" {
		t.Fatalf("the acknowledgement must precede the turn's first update, got %#v", frames[1])
	}
}

func TestACPv2LifecycleTurnEndsWithAnIdleStateUpdate(t *testing.T) {
	peer := authenticatedTestV2Peer(t, promptModeLifecycle)
	raw := captureStdout(t, func() { _, _ = peer.prompt(promptRequest(2, "hello")) })

	var updates []map[string]interface{}
	for _, frame := range decodeWireFrames(t, raw)[1:] {
		if frame["method"] != "session/update" {
			t.Fatalf("everything after the acknowledgement must be a session update, got %#v", frame)
		}
		params, _ := frame["params"].(map[string]interface{})
		update, _ := params["update"].(map[string]interface{})
		updates = append(updates, update)
	}
	if len(updates) == 0 {
		t.Fatal("the lifecycle turn must stream its work as session updates")
	}
	runningSeen := false
	for _, update := range updates {
		if update["sessionUpdate"] == "state_update" && update["state"] == "running" {
			runningSeen = true
		}
	}
	if !runningSeen {
		t.Fatalf("foreground work must report running, got %#v", updates)
	}
	last := updates[len(updates)-1]
	if last["sessionUpdate"] != "state_update" || last["state"] != "idle" || last["stopReason"] != "end_turn" {
		t.Fatalf("the turn must end with an idle state update carrying the stop reason, got %#v", last)
	}
	for _, update := range updates {
		if update["sessionUpdate"] == "agent_message_chunk" && update["messageId"] == nil {
			t.Fatalf("version 2 requires a message id on streamed chunks, got %#v", update)
		}
	}
}

func TestACPv2RichTurnCarriesEveryStreamingVariant(t *testing.T) {
	peer := authenticatedTestV2Peer(t, promptModeRich)
	peer.terminalDelay = 250 * time.Millisecond
	started := time.Now()
	raw := captureStdout(t, func() { _, _ = peer.prompt(promptRequest(2, "hello")) })
	elapsed := time.Since(started)

	seen := map[string]bool{}
	last := map[string]interface{}{}
	for _, frame := range decodeWireFrames(t, raw)[1:] {
		params, _ := frame["params"].(map[string]interface{})
		update, _ := params["update"].(map[string]interface{})
		if name, ok := update["sessionUpdate"].(string); ok {
			seen[name] = true
		}
		last = update
	}
	for _, variant := range []string{
		"user_message", "state_update", "plan_update", "agent_message_chunk",
		"tool_call_update", "tool_call_content_chunk", "agent_message",
		"terminal_update", "terminal_output_chunk",
	} {
		if !seen[variant] {
			t.Fatalf("the rich turn must exercise %s, saw %#v", variant, seen)
		}
	}
	if elapsed < 250*time.Millisecond {
		t.Fatalf("the terminal state must arrive after the configured delay, got %s", elapsed)
	}
	if last["state"] != "idle" || last["stopReason"] != "end_turn" {
		t.Fatalf("even a delayed turn ends with the terminal state, got %#v", last)
	}
}

func TestACPv2SilentPromptAcknowledgesAndStops(t *testing.T) {
	peer := authenticatedTestV2Peer(t, promptModeSilent)
	var resp jsonRPCResponse
	raw := captureStdout(t, func() { resp, _ = peer.prompt(promptRequest(2, "hello")) })

	if resp.Error != nil {
		t.Fatalf("the silent peer still acknowledges the prompt: %+v", resp.Error)
	}
	if frames := decodeWireFrames(t, raw); len(frames) != 0 {
		t.Fatalf("the silent mode must not stream anything, got %s", raw)
	}
}
