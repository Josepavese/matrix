package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
)

// ----------------------------------------------------------------------------
// Elicitation — single source of truth
// ----------------------------------------------------------------------------
//
// These neutral types are the authoritative Matrix definition of an
// elicitation: an agent-initiated request for structured user input. Protocol
// adapters (ACP today) project their wire forms into these types, and channel
// frontends (HTTP API today) consume them. Nothing protocol- or UI-specific
// may leak into this file.

// Elicitation modes. They mirror the stable ACP v1 elicitation surface, which
// Matrix adopted verbatim because the neutral model has no additional modes.
const (
	ElicitationModeForm = "form" // structured non-sensitive input
	ElicitationModeURL  = "url"  // out-of-band interaction, e.g. OAuth
)

// Elicitation actions, the three outcomes a frontend can return.
const (
	ElicitationActionAccept  = "accept"
	ElicitationActionDecline = "decline"
	ElicitationActionCancel  = "cancel"
)

// ElicitationOption is one allowed value of a constrained field, with the
// human label the agent supplied when it provided one.
type ElicitationOption struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
}

// ElicitationField is one form-mode question. The restricted shape (string,
// number, boolean, constrained choice) follows the stable ACP/MCP elicitation
// schema; anything richer is rejected at the projection boundary.
//
// Constrained choices arrive as JSON Schema enum or as oneOf/anyOf of
// const/title options — real agents use both, so both land in Options and
// nothing downstream needs to know which spelling the agent chose.
type ElicitationField struct {
	Name        string              `json:"name"`
	Title       string              `json:"title,omitempty"`
	Type        string              `json:"type"` // string | number | boolean | enum
	Description string              `json:"description,omitempty"`
	Required    bool                `json:"required,omitempty"`
	Options     []ElicitationOption `json:"options,omitempty"`
	Default     interface{}         `json:"default,omitempty"`
}

// OptionValues returns the allowed values in wire order, for consumers that
// only need the values and not the labels.
func (f ElicitationField) OptionValues() []string {
	values := make([]string, 0, len(f.Options))
	for _, option := range f.Options {
		values = append(values, option.Value)
	}
	return values
}

// ElicitationRequest is the neutral, protocol-agnostic elicitation request.
// Exactly one of Fields (form mode) or URL (url mode) is meaningful,
// selected by Mode. Scope mirrors ACP's flattened session/request binding.
type ElicitationRequest struct {
	// ID is the opaque, unique identifier a frontend answers with. It is
	// assigned by the frontend runtime, never by the agent.
	ID string `json:"id"`
	// AgentID identifies the agent asking. The stable spec requires the
	// client to clearly identify the requesting agent to the user, so this
	// field is mandatory in practice and empty only for malformed input.
	AgentID    string             `json:"agent_id,omitempty"`
	Mode       string             `json:"mode"`
	Message    string             `json:"message,omitempty"`
	Fields     []ElicitationField `json:"fields,omitempty"`
	URL        string             `json:"url,omitempty"`
	SessionID  string             `json:"session_id,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	RequestID  string             `json:"request_id,omitempty"`
	Meta       map[string]string  `json:"meta,omitempty"`
}

// ElicitationOutcome is the neutral answer to an ElicitationRequest. ID is
// filled by the frontend that resolved the request, so callers can correlate
// the answer with the pending entry the user actually saw.
type ElicitationOutcome struct {
	ID     string                 `json:"id,omitempty"`
	Action string                 `json:"action"` // accept | decline | cancel
	Values map[string]interface{} `json:"values,omitempty"`
}

// Accept builds the accept outcome for the given values.
func AcceptElicitation(values map[string]interface{}) ElicitationOutcome {
	return ElicitationOutcome{Action: ElicitationActionAccept, Values: values}
}

// DeclineElicitation is the explicit "user said no" outcome.
func DeclineElicitation() ElicitationOutcome {
	return ElicitationOutcome{Action: ElicitationActionDecline}
}

// CancelElicitation is the "interaction aborted" outcome (timeout, channel
// loss). Agents treat it like decline but may preserve partial work.
func CancelElicitation() ElicitationOutcome {
	return ElicitationOutcome{Action: ElicitationActionCancel}
}

// ----------------------------------------------------------------------------
// Validation — one implementation, every frontend
// ----------------------------------------------------------------------------

// ValidateElicitationValues checks submitted form values against the request's
// restricted schema. The stable spec requires clients to validate submitted
// values before responding, and agents re-validate afterwards; keeping the
// single implementation here means no frontend can drift from the schema it
// displayed. URL-mode requests carry no values and always validate.
func ValidateElicitationValues(req ElicitationRequest, values map[string]interface{}) error {
	if req.Mode != ElicitationModeForm {
		return nil
	}
	known := make(map[string]ElicitationField, len(req.Fields))
	for _, field := range req.Fields {
		known[field.Name] = field
	}
	for name := range values {
		if _, ok := known[name]; !ok {
			return fmt.Errorf("unknown field %q", name)
		}
	}
	for _, field := range req.Fields {
		value, present := values[field.Name]
		if !present || value == nil {
			if field.Required {
				return fmt.Errorf("field %q is required", field.Name)
			}
			continue
		}
		if err := validateElicitationValue(field, value); err != nil {
			return err
		}
	}
	return nil
}

func validateElicitationValue(field ElicitationField, value interface{}) error {
	switch field.Type {
	case "string":
		return validateStringValue(field, value)
	case "number":
		return validateNumberValue(field, value)
	case "boolean":
		return validateBooleanValue(field, value)
	case "enum":
		return validateEnumValue(field, value)
	}
	return nil
}

func validateStringValue(field ElicitationField, value interface{}) error {
	if _, ok := value.(string); !ok {
		return fmt.Errorf("field %q must be a string", field.Name)
	}
	return nil
}

// validateNumberValue rejects the values JSON cannot carry back to the agent:
// NaN and infinities would fail to encode after acceptance.
func validateNumberValue(field ElicitationField, value interface{}) error {
	finite := func(number float64) error {
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("field %q must be a finite number", field.Name)
		}
		return nil
	}
	switch typed := value.(type) {
	case float64:
		return finite(typed)
	case int, int64:
		return nil
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return fmt.Errorf("field %q must be a finite number", field.Name)
		}
		return finite(parsed)
	default:
		return fmt.Errorf("field %q must be a number", field.Name)
	}
}

func validateBooleanValue(field ElicitationField, value interface{}) error {
	if _, ok := value.(bool); !ok {
		return fmt.Errorf("field %q must be a boolean", field.Name)
	}
	return nil
}

func validateEnumValue(field ElicitationField, value interface{}) error {
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf("field %q must be a string", field.Name)
	}
	for _, allowed := range field.Options {
		if allowed.Value == text {
			return nil
		}
	}
	return fmt.Errorf("field %q must be one of %v", field.Name, field.OptionValues())
}

// ----------------------------------------------------------------------------
// Port — the elicitation frontend boundary (PAL)
// ----------------------------------------------------------------------------

// ElicitationFrontend resolves elicitation requests by surfacing them to a
// human. It is the only dependency the protocol layer may take on user
// interaction: the ACP adapter calls Ask and encodes whatever comes back.
//
// A nil frontend means elicitation is unsupported; adapters must then never
// advertise the capability and must answer inbound requests with a decline.
type ElicitationFrontend interface {
	// Ask blocks until the request is answered, declined, or the context
	// ends. Implementations must always return a usable outcome, never a
	// zero value with a nil error, and must set Outcome.ID to the pending
	// identifier when they assign request identity themselves.
	Ask(ctx context.Context, req ElicitationRequest) ElicitationOutcome
	// Modes lists the modes this frontend can actually serve, using the
	// ElicitationMode* constants. Advertisements are derived from this —
	// capability and reality cannot diverge.
	Modes() []string
}
