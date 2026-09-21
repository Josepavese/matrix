package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

// ----------------------------------------------------------------------------
// ACP elicitation projection
// ----------------------------------------------------------------------------
//
// This file is the single ACP↔neutral projection boundary for elicitation.
// Wire shapes live here; middleware.ElicitationRequest is the SSOT; the
// middleware.ElicitationFrontend port is the only user-interaction dependency.

// ErrCodeInvalidParams is JSON-RPC -32602, which the stable ACP elicitation
// spec mandates for requests using a mode the client did not advertise.

const ErrCodeInvalidParams = -32602

// acpElicitationCreateParams is the stable ACP v1 elicitation/create wire
// form. Form and URL fields are mutually exclusive by mode.

type acpElicitationCreateParams struct {
	SessionID       string                    `json:"sessionId,omitempty"`
	ToolCallID      string                    `json:"toolCallId,omitempty"`
	RequestID       flexibleRequestID         `json:"requestId,omitempty"`
	Mode            string                    `json:"mode,omitempty"`
	Message         string                    `json:"message,omitempty"`
	ElicitationID   string                    `json:"elicitationId,omitempty"`
	URL             string                    `json:"url,omitempty"`
	RequestedSchema *acpElicitationWireSchema `json:"requestedSchema,omitempty"`
}

type acpElicitationWireSchema struct {
	Type       string                                `json:"type,omitempty"`
	Properties map[string]acpElicitationWireProperty `json:"properties,omitempty"`
	Required   []string                              `json:"required,omitempty"`
}

type acpElicitationWireProperty struct {
	Type        string                     `json:"type,omitempty"`
	Title       string                     `json:"title,omitempty"`
	Description string                     `json:"description,omitempty"`
	Enum        []string                   `json:"enum,omitempty"`
	OneOf       []acpElicitationWireOption `json:"oneOf,omitempty"`
	AnyOf       []acpElicitationWireOption `json:"anyOf,omitempty"`
	Default     interface{}                `json:"default,omitempty"`
}

// acpElicitationWireOption is the const/title option shape real agents use for
// constrained choices, e.g. codex-acp approval scopes: once, session, always.

type acpElicitationWireOption struct {
	Const string   `json:"const,omitempty"`
	Title string   `json:"title,omitempty"`
	Enum  []string `json:"enum,omitempty"`
}

// flexibleRequestID accepts the request identifiers ACP peers actually send:
// the spec example is numeric, while JSON-RPC-derived peers send strings.
// Rejecting either would fail a conforming peer over a cosmetic difference.

type flexibleRequestID string

func (id *flexibleRequestID) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*id = flexibleRequestID(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("requestId must be a string or number")
	}
	*id = flexibleRequestID(number.String())
	return nil
}

func (id flexibleRequestID) String() string { return string(id) }

// wireToNeutralElicitation projects the ACP wire request into the neutral
// SSOT, enforcing the restricted elicitation schema: flat object, primitive
// or enum properties only. Anything richer is rejected, not coerced.

func neutralElicitationToWire(outcome middleware.ElicitationOutcome) map[string]interface{} {
	resp := map[string]interface{}{"action": outcome.Action}
	if outcome.Action == middleware.ElicitationActionAccept && len(outcome.Values) > 0 {
		resp["content"] = outcome.Values
	}
	return resp
}

// WithElicitationFrontend wires the neutral frontend port. The advertised
// capability and the served surface both derive from this one dependency.

func (h *defaultRequestHandler) WithElicitationFrontend(frontend middleware.ElicitationFrontend) *defaultRequestHandler {
	h.elicitation = frontend
	return h
}

// WithAgentIdentity records which agent this handler speaks for.

func (h *defaultRequestHandler) WithAgentIdentity(agentID string) *defaultRequestHandler {
	h.agentID = agentID
	return h
}

// elicitationAdvertisement derives the initialize-time capability from the
// port itself, so the advertised modes and the serving modes cannot diverge.

func elicitationAdvertisement(frontend middleware.ElicitationFrontend) *zedacp.ElicitationCapabilities {
	if frontend == nil {
		return nil
	}
	caps := &zedacp.ElicitationCapabilities{}
	for _, mode := range frontend.Modes() {
		switch mode {
		case middleware.ElicitationModeForm:
			caps.Form = &zedacp.ElicitationModeCapability{}
		case middleware.ElicitationModeURL:
			caps.URL = &zedacp.ElicitationModeCapability{}
		}
	}
	if caps.Form == nil && caps.URL == nil {
		return nil
	}
	return caps
}

// handleElicitationCreate answers agent-initiated elicitation/create requests
// by projecting to the neutral SSOT and delegating to the frontend port.
// Without a frontend the answer is always an explicit decline, so conforming
// agents run their documented fallback instead of waiting for a timeout.

func (h *defaultRequestHandler) handleElicitationCreate(ctx context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	var wire acpElicitationCreateParams
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &wire); err != nil {
			return nil, &zedacp.RPCError{Code: ErrCodeInvalidParams, Message: fmt.Sprintf("invalid elicitation/create params: %v", err)}
		}
	}
	req, err := wireToNeutralElicitation(wire, h.agentID)
	if err != nil {
		log.Warn("elicitation request rejected", "event", "elicitation_rejected", "error", err, "mode", wire.Mode)
		return nil, &zedacp.RPCError{Code: ErrCodeInvalidParams, Message: err.Error()}
	}
	if h.elicitation == nil {
		log.Info("elicitation request declined (no frontend)",
			"event", "elicitation_declined", "reason", "no_frontend",
			"session_id", req.SessionID, "mode", req.Mode)
		return neutralElicitationToWire(middleware.DeclineElicitation()), nil
	}
	if !modeAdvertised(h.elicitation, req.Mode) {
		log.Warn("elicitation request used unadvertised mode",
			"event", "elicitation_unadvertised_mode", "mode", req.Mode)
		return nil, &zedacp.RPCError{Code: ErrCodeInvalidParams, Message: fmt.Sprintf("elicitation mode %q was not advertised", req.Mode)}
	}
	req.ID = elicitationScopeKey(wire)
	log.Info("elicitation request forwarded to frontend",
		"event", "elicitation_forwarded",
		"scope_key", req.ID, "agent_id", req.AgentID, "session_id", req.SessionID,
		"mode", req.Mode, "fields", len(req.Fields), "message_len", len(req.Message))
	// Ask with the turn context when one is bound for this session, so a
	// cancelled run resolves the pending question immediately instead of
	// leaving a stale entry visible until the timeout.
	outcome := normalizeElicitationOutcome(h.elicitation.Ask(h.turnContextFor(ctx, req.SessionID), req), log, req)
	log.Info("elicitation resolved",
		"event", "elicitation_resolved",
		"elicitation_id", outcome.ID, "scope_key", req.ID,
		"agent_id", req.AgentID, "action", outcome.Action)
	return neutralElicitationToWire(outcome), nil
}

// normalizeElicitationOutcome keeps the protocol layer correct even when a
// frontend misbehaves. The stable surface admits exactly accept, decline and
// cancel; anything else — including the zero value a buggy or failed frontend
// returns — must become an explicit cancel rather than an invalid action the
// agent cannot interpret.

func normalizeElicitationOutcome(outcome middleware.ElicitationOutcome, log *slog.Logger, req middleware.ElicitationRequest) middleware.ElicitationOutcome {
	switch outcome.Action {
	case middleware.ElicitationActionAccept, middleware.ElicitationActionDecline, middleware.ElicitationActionCancel:
		return outcome
	default:
		log.Warn("elicitation frontend returned an invalid outcome; answering cancel",
			"event", "elicitation_invalid_outcome", "action", outcome.Action,
			"elicitation_id", outcome.ID, "scope_key", req.ID, "agent_id", req.AgentID)
		return middleware.ElicitationOutcome{ID: outcome.ID, Action: middleware.ElicitationActionCancel}
	}
}

func modeAdvertised(frontend middleware.ElicitationFrontend, mode string) bool {
	for _, supported := range frontend.Modes() {
		if supported == mode {
			return true
		}
	}
	return false
}

// elicitationScopeKey derives a human-readable correlation hint from ACP
// scope. It is only a hint: the registry owns request identity and makes it
// unique, because two questions can share one session scope.

func elicitationScopeKey(wire acpElicitationCreateParams) string {
	parts := make([]string, 0, 4)
	switch {
	case wire.SessionID != "":
		parts = append(parts, "session", wire.SessionID)
		if wire.ToolCallID != "" {
			parts = append(parts, "tool", wire.ToolCallID)
		}
	case wire.RequestID != "":
		parts = append(parts, "request", wire.RequestID.String())
	}
	if wire.ElicitationID != "" {
		parts = append(parts, "el", wire.ElicitationID)
	}
	if len(parts) == 0 {
		parts = append(parts, "ephemeral")
	}
	return strings.Join(parts, ":")
}
