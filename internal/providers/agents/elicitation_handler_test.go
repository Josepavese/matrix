package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// staticFrontend is a deterministic ElicitationFrontend for tests.
type staticFrontend struct {
	modes   []string
	asked   []middleware.ElicitationRequest
	outcome middleware.ElicitationOutcome
}

func (f *staticFrontend) Ask(_ context.Context, req middleware.ElicitationRequest) middleware.ElicitationOutcome {
	f.asked = append(f.asked, req)
	return f.outcome
}

func (f *staticFrontend) Modes() []string { return f.modes }

// TestHandleElicitationCreateDeclinesWithoutFrontend verifies the nil-port
// default: explicit decline, never a hang, and no capability advertisement.
func TestHandleElicitationCreateDeclinesWithoutFrontend(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)
	if got := elicitationAdvertisement(nil); got != nil {
		t.Fatalf("nil frontend must not advertise, got %+v", got)
	}
	result, err := handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(`{
		"sessionId": "sess_123", "mode": "form", "message": "Which database?",
		"requestedSchema": {"type": "object", "properties": {"db": {"type": "string", "enum": ["postgres", "sqlite"]}}, "required": ["db"]}
	}`))
	if err != nil {
		t.Fatalf("elicitation/create must not fail: %v", err)
	}
	encoded, _ := json.Marshal(result)
	var wire map[string]interface{}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["action"] != "decline" {
		t.Fatalf("expected decline, got %s", encoded)
	}
}

func TestHandleElicitationCreateForwardsToPort(t *testing.T) {
	frontend := &staticFrontend{
		modes:   []string{middleware.ElicitationModeForm},
		outcome: middleware.AcceptElicitation(map[string]interface{}{"strategy": "aggressive"}),
	}
	handler := newConfigurableRequestHandler(nil).WithElicitationFrontend(frontend)
	result, err := handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(`{
		"sessionId": "sess_9", "toolCallId": "call_1", "mode": "form",
		"message": "How should I approach this refactoring?",
		"requestedSchema": {"type": "object", "properties": {"strategy": {"type": "string", "enum": ["conservative", "balanced", "aggressive"]}}, "required": ["strategy"]}
	}`))
	if err != nil {
		t.Fatalf("forwarding failed: %v", err)
	}
	if len(frontend.asked) != 1 {
		t.Fatalf("frontend must be asked exactly once, got %d", len(frontend.asked))
	}
	req := frontend.asked[0]
	if req.SessionID != "sess_9" || req.ToolCallID != "call_1" || len(req.Fields) != 1 {
		t.Fatalf("projection lost scope or fields: %+v", req)
	}
	if req.Fields[0].Type != "enum" || !req.Fields[0].Required {
		t.Fatalf("enum field projection wrong: %+v", req.Fields[0])
	}
	encoded, _ := json.Marshal(result)
	var wire map[string]interface{}
	_ = json.Unmarshal(encoded, &wire)
	if wire["action"] != "accept" {
		t.Fatalf("expected accept, got %s", encoded)
	}
	content, ok := wire["content"].(map[string]interface{})
	if !ok || content["strategy"] != "aggressive" {
		t.Fatalf("expected content values on accept, got %s", encoded)
	}
}

func TestHandleElicitationCreateURLModeRequiresAbsoluteURL(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)
	_, err := handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(`{
		"requestId": 12, "mode": "url", "url": "not-a-url", "message": "authorize"
	}`))
	var rpcErr *zedacp.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32602 {
		t.Fatalf("expected -32602 for invalid url, got %v", err)
	}
}

func TestHandleElicitationCreateUnadvertisedMode(t *testing.T) {
	frontend := &staticFrontend{modes: []string{middleware.ElicitationModeForm}, outcome: middleware.DeclineElicitation()}
	handler := newConfigurableRequestHandler(nil).WithElicitationFrontend(frontend)
	_, err := handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(`{
		"requestId": 5, "mode": "url", "url": "https://agent.example/connect", "message": "authorize"
	}`))
	var rpcErr *zedacp.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32602 {
		t.Fatalf("expected -32602 for unadvertised mode, got %v", err)
	}
}

func TestHandleElicitationCreateRejectsRichSchema(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)
	_, err := handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(`{
		"sessionId": "s", "mode": "form",
		"requestedSchema": {"type": "object", "properties": {"nested": {"type": "object"}}}
	}`))
	var rpcErr *zedacp.RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected RPC error for unsupported property type, got %v", err)
	}
}

func TestHandleElicitationCreateRejectsMalformedParams(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)
	if _, err := handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(`{not-json`)); err == nil {
		t.Fatal("expected error for malformed elicitation params")
	}
}

func TestElicitationAdvertisementMirrorsFrontendModes(t *testing.T) {
	formOnly := &staticFrontend{modes: []string{middleware.ElicitationModeForm}}
	caps := elicitationAdvertisement(formOnly)
	if caps == nil || caps.Form == nil || caps.URL != nil {
		t.Fatalf("form-only frontend must advertise form only: %+v", caps)
	}
	none := &staticFrontend{modes: nil}
	if elicitationAdvertisement(none) != nil {
		t.Fatal("frontend with no modes must not advertise elicitation")
	}
}

// TestWireToNeutralElicitationAcceptsStringAndNumberRequestIDs covers both
// identifier shapes peers send: the spec example is numeric, JSON-RPC-derived
// peers send strings. Neither may fail a conforming peer.
func TestWireToNeutralElicitationAcceptsStringAndNumberRequestIDs(t *testing.T) {
	for _, raw := range []string{`12`, `"req-abc"`} {
		var wire acpElicitationCreateParams
		payload := `{"requestId": ` + raw + `, "mode": "url", "url": "https://agent.example/connect", "message": "authorize"}`
		if err := json.Unmarshal([]byte(payload), &wire); err != nil {
			t.Fatalf("requestId %s must decode: %v", raw, err)
		}
		req, err := wireToNeutralElicitation(wire, "claude")
		if err != nil {
			t.Fatalf("requestId %s must project: %v", raw, err)
		}
		if req.RequestID != strings.Trim(raw, `"`) {
			t.Fatalf("requestId %s projected as %q", raw, req.RequestID)
		}
		if req.AgentID != "claude" {
			t.Fatalf("agent identity must be propagated, got %q", req.AgentID)
		}
	}
}

// TestElicitationCarriesAgentIdentity asserts the stable spec requirement that
// a client can clearly identify the agent asking for information.
func TestElicitationCarriesAgentIdentity(t *testing.T) {
	frontend := &staticFrontend{
		modes:   []string{middleware.ElicitationModeForm},
		outcome: middleware.DeclineElicitation(),
	}
	handler := newConfigurableRequestHandler(nil).WithElicitationFrontend(frontend).WithAgentIdentity("gemini")
	_, err := handler.HandleRequest(context.Background(), "elicitation/create", json.RawMessage(`{
		"sessionId": "s1", "mode": "form", "message": "which?",
		"requestedSchema": {"type": "object", "properties": {"x": {"type": "string"}}}
	}`))
	if err != nil {
		t.Fatalf("elicitation failed: %v", err)
	}
	if len(frontend.asked) != 1 || frontend.asked[0].AgentID != "gemini" {
		t.Fatalf("frontend must receive the asking agent: %+v", frontend.asked)
	}
}

// TestWireToNeutralElicitationAcceptsOneOfOptions pins the real-world schema
// codex-acp sends for MCP tool approvals: the choice is expressed as oneOf of
// const/title options, with title and default, not as a plain enum. Dropping
// any of those would render a free-text box where the user must pick an
// approval scope.
func TestWireToNeutralElicitationAcceptsOneOfOptions(t *testing.T) {
	handler := newConfigurableRequestHandler(nil)
	params := json.RawMessage(`{
		"sessionId": "s1", "toolCallId": "exec-1", "mode": "form",
		"message": "Allow the mocksrv MCP server to run tool \"choose_database\"?",
		"requestedSchema": {
			"type": "object",
			"properties": {
				"persist": {
					"type": "string",
					"title": "Approval scope",
					"oneOf": [
						{"const": "once", "title": "Allow once"},
						{"const": "session", "title": "Allow for this session"},
						{"const": "always", "title": "Allow and don't ask again"}
					],
					"default": "once"
				}
			},
			"required": ["persist"]
		}
	}`)
	frontend := &staticFrontend{modes: []string{middleware.ElicitationModeForm}, outcome: middleware.DeclineElicitation()}
	handler.WithElicitationFrontend(frontend)
	if _, err := handler.HandleRequest(context.Background(), "elicitation/create", params); err != nil {
		t.Fatalf("oneOf approval schema must project: %v", err)
	}
	if len(frontend.asked) != 1 || len(frontend.asked[0].Fields) != 1 {
		t.Fatalf("expected one projected field, got %+v", frontend.asked)
	}
	field := frontend.asked[0].Fields[0]
	if field.Type != "enum" {
		t.Fatalf("oneOf must project as a constrained field, got %q", field.Type)
	}
	if field.Title != "Approval scope" {
		t.Fatalf("field title must survive projection, got %q", field.Title)
	}
	values := field.OptionValues()
	if len(values) != 3 || values[0] != "once" || values[2] != "always" {
		t.Fatalf("option values lost: %+v", field.Options)
	}
	if field.Options[2].Label != "Allow and don't ask again" {
		t.Fatalf("option labels lost: %+v", field.Options)
	}
	if field.Default != "once" {
		t.Fatalf("field default must survive projection, got %v", field.Default)
	}
	if err := middleware.ValidateElicitationValues(frontend.asked[0], map[string]interface{}{"persist": "always"}); err != nil {
		t.Fatalf("valid approval scope rejected: %v", err)
	}
	if err := middleware.ValidateElicitationValues(frontend.asked[0], map[string]interface{}{"persist": "postgres"}); err == nil {
		t.Fatal("invalid approval scope must be rejected")
	}
}

// TestWireToNeutralElicitationAcceptsAnyOfAndRejectsEmptyOption keeps the
// tolerant-but-bounded contract explicit.
func TestWireToNeutralElicitationAcceptsAnyOfAndRejectsEmptyOption(t *testing.T) {
	ok := acpElicitationWireProperty{Type: "string", AnyOf: []acpElicitationWireOption{{Const: "a", Title: "A"}}}
	options, err := wireOptions("field", ok)
	if err != nil || len(options) != 1 || options[0].Value != "a" || options[0].Label != "A" {
		t.Fatalf("anyOf must project: %+v err=%v", options, err)
	}
	bad := acpElicitationWireProperty{Type: "string", OneOf: []acpElicitationWireOption{{Title: "no value"}}}
	if _, err := wireOptions("field", bad); err == nil {
		t.Fatal("an option without a const value must be rejected")
	}
}
