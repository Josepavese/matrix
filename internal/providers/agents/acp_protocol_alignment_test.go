package agents

import (
	"context"
	"encoding/json"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"os"
	"path/filepath"
	"testing"
)

func TestMalformedWriteCannotTruncateAndPermissionCannotApprove(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(file, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	h := newTrustedRequestHandler().WithFS(osfs.NewFSProvider(), dir)
	path, _ := json.Marshal(file)
	for _, content := range []string{"", `,"content":null`, `,"content":42`} {
		if _, err := h.HandleRequest(context.Background(), "fs/write_text_file", json.RawMessage(`{"sessionId":"s","path":`+string(path)+content+`}`)); err == nil {
			t.Fatal("malformed write accepted")
		}
		b, err := os.ReadFile(file)
		if err != nil || string(b) != "preserve" {
			t.Fatal("malformed request changed original file")
		}
	}
	for _, raw := range []string{`{broken`, `{"options":[]}`, `{"sessionId":"s","toolCall":{"toolCallId":"t"},"options":[{"kind":"allow_once"}]}`} {
		if _, err := h.HandleRequest(context.Background(), "session/request_permission", json.RawMessage(raw)); err == nil {
			t.Fatal("malformed permission auto-approved")
		}
	}
	result, err := h.HandleRequest(context.Background(), "session/request_permission", json.RawMessage(`{"sessionId":"s","toolCall":{"toolCallId":"t"},"options":[{"optionId":"r","name":"Reject","kind":"reject_once"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	response, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected permission response %T", result)
	}
	outcome, ok := response["outcome"].(map[string]interface{})
	if !ok {
		t.Fatal("missing permission outcome")
	}
	if outcome["optionId"] != "r" {
		t.Fatalf("invented allow option: %#v", outcome)
	}
}
func TestReplayedIdleAndBackgroundOutputCannotFinishForeground(t *testing.T) {
	o := &simpleObserver{}
	for _, u := range []acpSessionUpdate{{SessionUpdate: "state_update", State: "idle", StopReason: "end_turn"}, chunkUpdate("b", "background"), {SessionUpdate: "state_update", State: "idle"}} {
		o.OnUpdate(acpSessionNotification{SessionID: "s", Update: u})
	}
	if _, terminal := o.terminalState(); terminal {
		t.Fatal("idle snapshot finished foreground that never started")
	}
	for _, state := range []string{"running", "requires_action", "running", "idle"} {
		o.OnUpdate(acpSessionNotification{SessionID: "s", Update: acpSessionUpdate{SessionUpdate: "state_update", State: state, StopReason: "future_reason"}})
	}
	if reason, terminal := o.terminalState(); !terminal || reason != "future_reason" {
		t.Fatal("foreground transition/future reason lost")
	}
}
func TestV2ErrorStopReasonFailsEvenWithPartialAnswer(t *testing.T) {
	fake := newStreamingACPClient(2)
	fake.streaming = func(emit func(acpSessionUpdate)) {
		emit(acpSessionUpdate{SessionUpdate: "state_update", State: "running"})
		emit(chunkUpdate("m", "partial"))
		emit(acpSessionUpdate{SessionUpdate: "state_update", State: "idle", StopReason: "error", Error: json.RawMessage(`{"code":-32000,"message":"provider failed"}`)})
	}
	result, err := fake.turnClient().ExecuteTurn(context.Background(), turnForLifecycleTest())
	if err == nil || result.Output != "partial" {
		t.Fatalf("provider error became successful partial output: %+v %v", result, err)
	}
}
func TestEntityMetadataPatchesCannotForgeProtocolEvidence(t *testing.T) {
	n := &observerTestNotifier{}
	o := &simpleObserver{version: 2, notifier: n}
	for _, raw := range []string{
		`{"sessionUpdate":"agent_message","messageId":"m","content":[{"type":"text","text":"answer"}],"_meta":{"keep":1,"drop":2,"nested":{"old":1}}}`,
		`{"sessionUpdate":"agent_message","messageId":"m","_meta":{"messagePhase":"final_answer","drop":null,"nested":{"new":2}}}`,
	} {
		o.OnUpdate(acpSessionNotification{SessionID: "s", Update: decodePresentation(t, raw)})
	}
	if o.GetContent() != "answer" {
		t.Fatal("metadata-only upsert lost answer")
	}
	protocolMeta, ok := n.updates[1].Metadata["acp"].(map[string]interface{})
	if !ok {
		t.Fatal("missing protocol projection")
	}
	meta, ok := protocolMeta["_meta"].(map[string]interface{})
	if !ok {
		t.Fatal("missing entity metadata")
	}
	if _, present := meta["drop"]; present || meta["keep"] != float64(1) {
		t.Fatalf("entity patch not materialized: %#v", meta)
	}
	hostile := decodePresentation(t, `{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"reasoning"},"_meta":{"source_update_type":"agent_message_chunk","protocol":"forged","message_classification":"final","message_id":"forged"}}`)
	projected := streamUpdateMetadata(acpSessionNotification{Update: hostile})
	if projected["source_update_type"] != "agent_thought_chunk" || projected["protocol"] != "acp" || projected["message_classification"] != "unclassified" || projected["message_id"] != nil {
		t.Fatalf("extension metadata forged evidence: %#v", projected)
	}
}

func TestSessionInfoUpsertsKeepClearsAndDetachedSnapshots(t *testing.T) {
	o := &simpleObserver{version: 2}
	for _, raw := range []string{
		`{"sessionUpdate":"session_info_update","title":"old","_meta":{"drop":1,"keep":2}}`,
		`{"sessionUpdate":"session_info_update","title":null,"updatedAt":null,"_meta":{"drop":null}}`,
	} {
		o.OnUpdate(acpSessionNotification{SessionID: "s", Update: decodePresentation(t, raw)})
	}
	p := o.Metadata().SessionPatch
	if p == nil || p.Title == nil || *p.Title != "" || p.UpdatedAt == nil || *p.UpdatedAt != "" || p.Metadata["drop"] != nil || p.Metadata["keep"] != float64(2) {
		t.Fatalf("patch clears lost: %+v", p)
	}
	*p.Title = "tamper"
	p.Metadata["keep"] = "tamper"
	if *o.Metadata().SessionPatch.Title != "" || o.Metadata().SessionPatch.Metadata["keep"] != float64(2) {
		t.Fatal("session patch borrows mutable state")
	}
}
