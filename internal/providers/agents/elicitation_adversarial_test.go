package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// ----------------------------------------------------------------------------
// Adversarial projection suite
// ----------------------------------------------------------------------------
//
// The wire is untrusted input: an agent can send anything. Every case here is
// either a rejection that must stay typed (-32602) or a tolerated shape that
// must survive projection without panicking or losing meaning.

func mustRejectInvalidParams(t *testing.T, payload string) {
	t.Helper()
	handler := newConfigurableRequestHandler(nil)
	_, err := handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(payload))
	var rpcErr *zedacp.RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("payload %s: expected a typed JSON-RPC error, got %v", payload, err)
	}
	if rpcErr.Code != ErrCodeInvalidParams {
		t.Fatalf("payload %s: expected -32602, got %d", payload, rpcErr.Code)
	}
}

func TestProjectionRejectsHostileInputs(t *testing.T) {
	cases := map[string]string{
		"not json":                   `{`,
		"json array":                 `[1,2,3]`,
		"json string":                `"hello"`,
		"json number":                `42`,
		"unknown mode":               `{"mode":"sms","message":"hi"}`,
		"empty mode":                 `{"mode":"","message":"hi"}`,
		"form without schema":        `{"mode":"form","message":"hi"}`,
		"form with null schema":      `{"mode":"form","requestedSchema":null}`,
		"form with empty properties": `{"mode":"form","requestedSchema":{"type":"object","properties":{}}}`,
		"schema not an object":       `{"mode":"form","requestedSchema":{"type":"array","properties":{"a":{"type":"string"}}}}`,
		"object property":            `{"mode":"form","requestedSchema":{"type":"object","properties":{"nested":{"type":"object"}}}}`,
		"array property":             `{"mode":"form","requestedSchema":{"type":"object","properties":{"list":{"type":"array"}}}}`,
		"null property type":         `{"mode":"form","requestedSchema":{"type":"object","properties":{"x":{"type":null}}}}`,
		"enum on a number field":     `{"mode":"form","requestedSchema":{"type":"object","properties":{"x":{"type":"number","enum":["a"]}}}}`,
		"oneOf without const":        `{"mode":"form","requestedSchema":{"type":"object","properties":{"x":{"type":"string","oneOf":[{"title":"no value"}]}}}}`,
		"oneOf with multi enum":      `{"mode":"form","requestedSchema":{"type":"object","properties":{"x":{"type":"string","oneOf":[{"enum":["a","b"]}]}}}}`,
		"url mode without url":       `{"mode":"url","message":"go"}`,
		"url mode with relative url": `{"mode":"url","url":"/oauth","message":"go"}`,
		"url mode with javascript":   `{"mode":"url","url":"javascript:alert(1)","message":"go"}`,
		"url mode with data uri":     `{"mode":"url","url":"data:text/html,<script>","message":"go"}`,
		"url mode with file scheme":  `{"mode":"url","url":"file:///etc/passwd","message":"go"}`,
		"url mode with empty host":   `{"mode":"url","url":"https://","message":"go"}`,
		"requestId of wrong type":    `{"requestId":{"nested":true},"mode":"url","url":"https://x.example","message":"go"}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) { mustRejectInvalidParams(t, payload) })
	}
}

func TestProjectionToleratesLegitimateShapes(t *testing.T) {
	cases := map[string]struct {
		payload    string
		wantFields int
		wantMode   string
	}{
		"minimal form": {
			payload:    `{"mode":"form","requestedSchema":{"type":"object","properties":{"a":{"type":"string"}}}}`,
			wantFields: 1, wantMode: middleware.ElicitationModeForm,
		},
		"schema without explicit type": {
			payload:    `{"mode":"form","requestedSchema":{"properties":{"a":{"type":"boolean"}}}}`,
			wantFields: 1, wantMode: middleware.ElicitationModeForm,
		},
		"extra json schema keywords": {
			payload:    `{"mode":"form","requestedSchema":{"type":"object","additionalProperties":false,"properties":{"a":{"type":"string","minLength":2,"format":"email"}}}}`,
			wantFields: 1, wantMode: middleware.ElicitationModeForm,
		},
		"number and boolean fields": {
			payload:    `{"mode":"form","requestedSchema":{"type":"object","properties":{"n":{"type":"number"},"b":{"type":"boolean"}}}}`,
			wantFields: 2, wantMode: middleware.ElicitationModeForm,
		},
		"url mode with query and port": {
			payload:    `{"mode":"url","url":"https://auth.example.com:8443/oauth?x=1#frag","message":"go"}`,
			wantFields: 0, wantMode: middleware.ElicitationModeURL,
		},
		"string request id": {
			payload:    `{"requestId":"stream-7","mode":"url","url":"https://x.example","message":"go"}`,
			wantFields: 0, wantMode: middleware.ElicitationModeURL,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var wire acpElicitationCreateParams
			if err := json.Unmarshal([]byte(tc.payload), &wire); err != nil {
				t.Fatalf("payload must decode: %v", err)
			}
			request, err := wireToNeutralElicitation(wire, "codex")
			if err != nil {
				t.Fatalf("legitimate shape rejected: %v", err)
			}
			if len(request.Fields) != tc.wantFields || request.Mode != tc.wantMode {
				t.Fatalf("projection lost meaning: %+v", request)
			}
		})
	}
}

// TestProjectionSurvivesDuplicateFieldNames pins duplicate handling: JSON
// decoding collapses duplicates (last wins) and the surviving field is usable.
func TestProjectionSurvivesDuplicateFieldNames(t *testing.T) {
	wire := acpElicitationCreateParams{}
	payload := `{"mode":"form","requestedSchema":{"type":"object","properties":{"dup":{"type":"string"},"dup":{"type":"number"}}}}`
	if err := json.Unmarshal([]byte(payload), &wire); err != nil {
		t.Fatalf("decode: %v", err)
	}
	request, err := wireToNeutralElicitation(wire, "codex")
	if err != nil {
		t.Fatalf("degenerate but parseable schema must not fail the request: %v", err)
	}
	if len(request.Fields) != 1 {
		t.Fatalf("expected duplicate collapse to one field, got %+v", request.Fields)
	}
	if request.Fields[0].Name != "dup" || request.Fields[0].Type != "number" {
		t.Fatalf("duplicate key must follow JSON last-wins semantics: %+v", request.Fields[0])
	}
}

// TestProjectionRejectsUnnamedProperty is the fuzz regression: the projection
// used to accept a property named "" and produce a field no frontend can label.
func TestProjectionRejectsUnnamedProperty(t *testing.T) {
	wire := acpElicitationCreateParams{}
	payload := `{"mode":"form","requestedSchema":{"type":"object","properties":{"":{"type":"string"}}}}`
	if err := json.Unmarshal([]byte(payload), &wire); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err := wireToNeutralElicitation(wire, "codex"); err == nil {
		t.Fatal("a property without a name must be rejected")
	}
}

// TestProjectionPreservesLargeSchema keeps a pathological but legal form from
// losing fields or panicking.
func TestProjectionPreservesLargeSchema(t *testing.T) {
	properties := make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		properties = append(properties, fmt.Sprintf(`"f%d":{"type":"string"}`, i))
	}
	payload := `{"mode":"form","requestedSchema":{"type":"object","properties":{` + strings.Join(properties, ",") + `}}}`
	var wire acpElicitationCreateParams
	if err := json.Unmarshal([]byte(payload), &wire); err != nil {
		t.Fatal(err)
	}
	request, err := wireToNeutralElicitation(wire, "codex")
	if err != nil {
		t.Fatalf("large schema rejected: %v", err)
	}
	if len(request.Fields) != 60 {
		t.Fatalf("expected 60 fields, got %d", len(request.Fields))
	}
}

// TestProjectionDoesNotLeakURLCredentials checks the display path: a URL with
// embedded userinfo must never surface the password as the host to show.
func TestProjectionDoesNotLeakURLCredentials(t *testing.T) {
	var wire acpElicitationCreateParams
	payload := `{"mode":"url","url":"https://user:secret@auth.example.com/oauth","message":"go"}`
	if err := json.Unmarshal([]byte(payload), &wire); err != nil {
		t.Fatal(err)
	}
	request, err := wireToNeutralElicitation(wire, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if request.URL != "https://user:secret@auth.example.com/oauth" {
		t.Fatalf("url must cross the neutral layer unchanged: %q", request.URL)
	}
}

// ----------------------------------------------------------------------------
// Handler-level invariants
// ----------------------------------------------------------------------------

// TestHandlerAlwaysAnswersWithAProtocolResult is the no-hang invariant: every
// inbound request must produce a JSON-RPC result or a typed error, whatever the
// frontend does — including a frontend that returns a zero outcome.
func TestHandlerAlwaysAnswersWithAProtocolResult(t *testing.T) {
	frontend := &staticFrontend{modes: []string{middleware.ElicitationModeForm}}
	handler := newConfigurableRequestHandler(nil).WithElicitationFrontend(frontend).WithAgentIdentity("codex")
	result, err := handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(`{
		"sessionId":"s","mode":"form","message":"q",
		"requestedSchema":{"type":"object","properties":{"a":{"type":"string"}}}
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	encoded, _ := json.Marshal(result)
	var wire map[string]interface{}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	action, _ := wire["action"].(string)
	switch action {
	case "accept", "decline", "cancel":
	default:
		t.Fatalf("handler produced an invalid action %q: %s", action, encoded)
	}
}

// TestHandlerRejectsNilFrontendModesSafely covers a frontend that advertises no
// usable mode: the request must be refused as unadvertised, not answered.
func TestHandlerRejectsNilFrontendModesSafely(t *testing.T) {
	frontend := &staticFrontend{modes: nil, outcome: middleware.DeclineElicitation()}
	handler := newConfigurableRequestHandler(nil).WithElicitationFrontend(frontend)
	_, err := handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(`{
		"mode":"form","requestedSchema":{"type":"object","properties":{"a":{"type":"string"}}}
	}`))
	var rpcErr *zedacp.RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected -32602 for an unadvertised mode, got %v", err)
	}
}

// TestHandlerIsSafeUnderConcurrentRequests exercises the per-session turn map
// and the shared frontend from many goroutines at once.
func TestHandlerIsSafeUnderConcurrentRequests(t *testing.T) {
	service := elicitation.NewService(0)
	handler := newConfigurableRequestHandler(nil).WithElicitationFrontend(service).WithAgentIdentity("codex")

	// beginPrompt serialises turns per remote session, so concurrency here is
	// across sessions: one live turn per session is the real invariant.
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			session := fmt.Sprintf("s%d", index)
			turnCtx, cancel := context.WithCancel(context.Background())
			handler.BindTurnContext(turnCtx, session)
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(fmt.Sprintf(`{
					"sessionId":%q,"mode":"form","message":"q",
					"requestedSchema":{"type":"object","properties":{"a":{"type":"string"}}}
				}`, session)))
			}()
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if len(service.Pending()) >= 1 && pendingHasSession(service, session) {
					break
				}
				time.Sleep(time.Millisecond)
			}
			cancel()
			<-done
			handler.ClearTurnContext(session)
		}(i)
	}
	wg.Wait()
	if pending := service.Pending(); len(pending) != 0 {
		t.Fatalf("cancelled turns left %d pending elicitations", len(pending))
	}
}

// TestHandlerRejectsUnknownElicitationMethods keeps the extension boundary.
func TestHandlerRejectsUnknownElicitationMethods(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)
	_, err := handler.HandleRequest(context.Background(), "elicitation/complete", json.RawMessage(`{}`))
	var rpcErr *zedacp.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != zedacp.ErrCodeMethodNotFound {
		t.Fatalf("unknown method must be method-not-found, got %v", err)
	}
}

// pendingHasSession reports whether the registry holds a request for a session.
func pendingHasSession(service *elicitation.Service, sessionID string) bool {
	for _, pending := range service.Pending() {
		if pending.Request.SessionID == sessionID {
			return true
		}
	}
	return false
}
