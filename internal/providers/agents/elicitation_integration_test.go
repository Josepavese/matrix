package agents

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
)

// TestElicitationEndToEndThroughRegistry proves the layers actually talk:
// the ACP adapter projects a wire request into the real registry frontend,
// blocks, and the answer posted by the HTTP channel frontend (the same
// Service instance) comes back out as a stable ACP response.
func TestElicitationEndToEndThroughRegistry(t *testing.T) {
	service := elicitation.NewService(5 * time.Second)
	handler := newConfigurableRequestHandler(nil).
		WithElicitationFrontend(service).
		WithAgentIdentity("opencode")

	type result struct {
		payload interface{}
		err     error
	}
	done := make(chan result, 1)
	go func() {
		payload, err := handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(`{
			"sessionId": "sess_e2e", "mode": "form",
			"message": "Which database should I use?",
			"requestedSchema": {
				"type": "object",
				"properties": {"db": {"type": "string", "enum": ["postgres", "sqlite"]}},
				"required": ["db"]
			}
		}`))
		done <- result{payload: payload, err: err}
	}()

	// The channel frontend sees exactly what the agent asked.
	var pending middleware.ElicitationRequest
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		entries := service.Pending()
		if len(entries) == 1 {
			pending = entries[0].Request
			break
		}
		time.Sleep(time.Millisecond)
	}
	if pending.ID == "" {
		t.Fatal("ACP request never reached the shared registry")
	}
	if pending.AgentID != "opencode" {
		t.Fatalf("registry lost the asking agent: %+v", pending)
	}
	if len(pending.Fields) != 1 || pending.Fields[0].Name != "db" || !pending.Fields[0].Required {
		t.Fatalf("registry lost the form schema: %+v", pending.Fields)
	}

	// Answering through the registry resolves the blocked ACP request.
	if !service.Respond(pending.ID, middleware.AcceptElicitation(map[string]interface{}{"db": "postgres"})) {
		t.Fatal("registry refused a pending answer")
	}
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("ACP handler failed: %v", got.err)
		}
		encoded, _ := json.Marshal(got.payload)
		var wire map[string]interface{}
		if err := json.Unmarshal(encoded, &wire); err != nil {
			t.Fatal(err)
		}
		if wire["action"] != "accept" {
			t.Fatalf("expected accept on the wire, got %s", encoded)
		}
		content, ok := wire["content"].(map[string]interface{})
		if !ok || content["db"] != "postgres" {
			t.Fatalf("answer did not reach the agent: %s", encoded)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ACP handler never returned after the answer")
	}
}

// TestACPDepsAdvertiseElicitationOnlyWithFrontend pins the initialize-time
// capability to the port, so the advertised surface can never exceed reality.
func TestACPDepsAdvertiseElicitationOnlyWithFrontend(t *testing.T) {
	without := acpClientCapabilitiesForDeps(middleware.ConversationFactoryDeps{})
	if without.Elicitation != nil {
		t.Fatalf("no frontend must keep elicitation unadvertised: %+v", without.Elicitation)
	}
	frontend := &staticFrontend{modes: []string{middleware.ElicitationModeForm, middleware.ElicitationModeURL}}
	with := acpClientCapabilitiesForDeps(middleware.ConversationFactoryDeps{ElicitationFrontend: frontend})
	if with.Elicitation == nil || with.Elicitation.Form == nil || with.Elicitation.URL == nil {
		t.Fatalf("frontend modes must be advertised: %+v", with.Elicitation)
	}
	encoded, err := json.Marshal(with)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]interface{}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	elicitationWire, ok := wire["elicitation"].(map[string]interface{})
	if !ok {
		t.Fatalf("wire capabilities must carry an elicitation object: %s", encoded)
	}
	if _, ok := elicitationWire["form"]; !ok {
		t.Fatalf("wire shape must carry an explicit form object, per spec: %s", encoded)
	}
	if _, ok := elicitationWire["url"]; !ok {
		t.Fatalf("wire shape must carry an explicit url object, per spec: %s", encoded)
	}
}

// TestElicitationIsRevokedWhenTheTurnIsCancelled covers the run-cancel case: a
// question asked mid-run must disappear from the pending surface as soon as
// the run is cancelled, instead of lingering until the elicitation timeout and
// being answered for a turn that no longer exists.
func TestElicitationIsRevokedWhenTheTurnIsCancelled(t *testing.T) {
	service := elicitation.NewService(time.Hour) // long timeout: only cancellation can resolve it
	handler := newConfigurableRequestHandler(nil).
		WithElicitationFrontend(service).
		WithAgentIdentity("codex")

	turnCtx, cancelTurn := context.WithCancel(context.Background())
	handler.BindTurnContext(turnCtx, "sess_cancel")

	type result struct {
		payload interface{}
		err     error
	}
	done := make(chan result, 1)
	go func() {
		payload, err := handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(`{
			"sessionId": "sess_cancel", "mode": "form",
			"message": "Which database?",
			"requestedSchema": {"type": "object", "properties": {"db": {"type": "string"}}, "required": ["db"]}
		}`))
		done <- result{payload: payload, err: err}
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(service.Pending()) == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(service.Pending()) != 1 {
		t.Fatal("elicitation never became pending")
	}

	cancelTurn()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("cancelled turn must still produce a protocol answer: %v", got.err)
		}
		encoded, _ := json.Marshal(got.payload)
		var wire map[string]interface{}
		if err := json.Unmarshal(encoded, &wire); err != nil {
			t.Fatal(err)
		}
		if wire["action"] != "cancel" {
			t.Fatalf("cancelled run must answer cancel, got %s", encoded)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelling the run left the elicitation blocked until timeout")
	}
	if pending := service.Pending(); len(pending) != 0 {
		t.Fatalf("cancelled run left %d stale pending elicitation(s): %+v", len(pending), pending)
	}
}

// TestTurnContextBindingIsPerSession proves the fallback contract: a session
// without a bound turn keeps the connection context, and clearing a binding
// restores the fallback for that session only.
func TestTurnContextBindingIsPerSession(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)
	// Distinct values so identity comparison is meaningful: context.Background()
	// is a singleton shared by every caller.
	fallback, cancelFallback := context.WithCancel(context.Background())
	defer cancelFallback()
	sessionOne, cancelOne := context.WithCancel(context.Background())
	defer cancelOne()
	sessionTwo, cancelTwo := context.WithCancel(context.Background())
	defer cancelTwo()

	if handler.turnContextFor(fallback, "s1") != fallback {
		t.Fatal("unbound session must use the connection context")
	}
	handler.BindTurnContext(sessionOne, "s1")
	handler.BindTurnContext(sessionTwo, "s2")
	if handler.turnContextFor(fallback, "s1") != sessionOne {
		t.Fatal("bound session must use its turn context")
	}
	if handler.turnContextFor(fallback, "s3") != fallback {
		t.Fatal("other sessions must keep the fallback")
	}
	handler.ClearTurnContext("s1")
	if handler.turnContextFor(fallback, "s1") != fallback {
		t.Fatal("cleared session must fall back to the connection context")
	}
	if handler.turnContextFor(fallback, "s2") != sessionTwo {
		t.Fatal("clearing one session must not clear another")
	}
	handler.BindTurnContext(sessionOne, "")
	if handler.turnContextFor(fallback, "s1") != fallback {
		t.Fatal("empty session ids must not create bindings")
	}
}
