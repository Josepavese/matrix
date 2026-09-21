package agents

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
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

func wireToNeutralElicitation(wire acpElicitationCreateParams, agentID string) (middleware.ElicitationRequest, error) {
	req := middleware.ElicitationRequest{
		AgentID:    agentID,
		Mode:       wire.Mode,
		Message:    wire.Message,
		SessionID:  wire.SessionID,
		ToolCallID: wire.ToolCallID,
	}
	if id := wire.RequestID.String(); id != "" {
		req.RequestID = id
	}
	switch wire.Mode {
	case middleware.ElicitationModeURL:
		return projectURLMode(req, wire)
	case middleware.ElicitationModeForm:
		return projectFormMode(req, wire)
	default:
		return req, fmt.Errorf("elicitation mode %q is not one of form, url", wire.Mode)
	}
}

// projectURLMode validates a url-mode request. Matrix never opens the URL itself;
// it asks the human to open it, so the destination must be a web page. Arbitrary
// schemes (file, smb, custom handlers, and the "A://" style values seen from
// fuzzing) would let an agent steer a browser or a handler outside the web.

func projectURLMode(req middleware.ElicitationRequest, wire acpElicitationCreateParams) (middleware.ElicitationRequest, error) {
	parsed, err := url.Parse(wire.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return req, fmt.Errorf("elicitation url mode requires an absolute URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return req, fmt.Errorf("elicitation url mode requires an http or https URL, got %q", parsed.Scheme)
	}
	req.URL = wire.URL
	req.Meta = map[string]string{"elicitation_id": wire.ElicitationID}
	return req, nil
}

// projectFormMode validates the restricted form schema and projects every
// property. The schema must be a flat object with at least one named property.

func projectFormMode(req middleware.ElicitationRequest, wire acpElicitationCreateParams) (middleware.ElicitationRequest, error) {
	schema := wire.RequestedSchema
	if schema == nil {
		return req, fmt.Errorf("elicitation form mode requires requestedSchema")
	}
	if schema.Type != "" && schema.Type != "object" {
		return req, fmt.Errorf("elicitation form schema must be a flat object")
	}
	if len(schema.Properties) == 0 {
		return req, fmt.Errorf("elicitation form schema requires at least one property")
	}
	required := map[string]bool{}
	for _, name := range schema.Required {
		required[name] = true
	}
	fields := make([]middleware.ElicitationField, 0, len(schema.Properties))
	for _, name := range sortedPropertyNames(schema.Properties) {
		field, err := projectFormField(name, schema.Properties[name], required[name])
		if err != nil {
			return req, err
		}
		fields = append(fields, field)
	}
	req.Fields = fields
	return req, nil
}

// projectFormField projects one property, rejecting anything the restricted
// schema does not cover rather than coercing it.

func projectFormField(name string, prop acpElicitationWireProperty, required bool) (middleware.ElicitationField, error) {
	if strings.TrimSpace(name) == "" {
		// A property without a name cannot be labelled for the human nor keyed in
		// the answer the agent receives, so the schema is malformed.
		return middleware.ElicitationField{}, fmt.Errorf("elicitation property names must not be empty")
	}
	options, err := wireOptions(name, prop)
	if err != nil {
		return middleware.ElicitationField{}, err
	}
	fieldType := prop.Type
	if len(options) > 0 {
		if fieldType != "" && fieldType != "string" {
			return middleware.ElicitationField{}, fmt.Errorf("elicitation property %s: a constrained choice must be a string field, got %q", name, prop.Type)
		}
		fieldType = "enum"
	}
	switch fieldType {
	case "string", "number", "boolean", "enum":
	default:
		return middleware.ElicitationField{}, fmt.Errorf("elicitation property %s: unsupported type %q (allowed: string, number, boolean, enum)", name, prop.Type)
	}
	return middleware.ElicitationField{
		Name:        name,
		Title:       prop.Title,
		Type:        fieldType,
		Description: prop.Description,
		Required:    required,
		Options:     options,
		Default:     prop.Default,
	}, nil
}

// sortedPropertyNames returns the schema's property names in a stable order, so
// the same request always renders the same form.

func sortedPropertyNames(properties map[string]acpElicitationWireProperty) []string {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// wireOptions converts every constrained-choice spelling an agent may use into
// the single neutral option list: JSON Schema enum, or oneOf/anyOf of
// const/title (and single-value enum) options. Labels are preserved because the
// agent's wording is what the user must choose between.

func wireOptions(name string, prop acpElicitationWireProperty) ([]middleware.ElicitationOption, error) {
	if len(prop.Enum) > 0 {
		options := make([]middleware.ElicitationOption, 0, len(prop.Enum))
		for _, value := range prop.Enum {
			options = append(options, middleware.ElicitationOption{Value: value})
		}
		return options, nil
	}
	source := prop.OneOf
	if len(source) == 0 {
		source = prop.AnyOf
	}
	if len(source) == 0 {
		return nil, nil
	}
	options := make([]middleware.ElicitationOption, 0, len(source))
	for _, option := range source {
		value := option.Const
		if value == "" && len(option.Enum) == 1 {
			value = option.Enum[0]
		}
		if value == "" {
			return nil, fmt.Errorf("elicitation property %s: oneOf/anyOf options must carry a const value", name)
		}
		options = append(options, middleware.ElicitationOption{Value: value, Label: option.Title})
	}
	return options, nil
}

// neutralElicitationToWire encodes the neutral outcome into the stable ACP
// response shape: {action, content?}. content is only sent on accept with
// values, matching the spec's omission semantics.
