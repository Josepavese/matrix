package agents

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// ----------------------------------------------------------------------------
// Fuzz targets for the ACP elicitation wire projection
// ----------------------------------------------------------------------------
//
// This is the exact bytes an agent sends to ask the human a question. The
// projection decides what the human is shown and what values the agent will
// accept back, so a panic here hangs a real conversation and a coercion mistake
// here lets an answer through that the schema forbids.

// FuzzWireToNeutralElicitation asserts that the projection is total: it either
// returns a typed error or a request whose fields are internally consistent and
// re-validatable.
func FuzzWireToNeutralElicitation(f *testing.F) {
	seeds := []string{
		`{"mode":"form","message":"q","requestedSchema":{"type":"object","properties":{"a":{"type":"string"}}}}`,
		`{"mode":"form","requestedSchema":{"type":"object","properties":{"n":{"type":"number"},"b":{"type":"boolean"}}}}`,
		`{"mode":"form","requestedSchema":{"type":"object","properties":{"e":{"type":"string","enum":["a","b"]}}}}`,
		`{"mode":"form","requestedSchema":{"type":"object","properties":{"o":{"type":"string","oneOf":[{"const":"x","title":"X"}]}}}}`,
		`{"mode":"form","requestedSchema":{"type":"object","properties":{"o":{"type":"string","anyOf":[{"const":"x"},{"const":"y"}]}}}}`,
		`{"mode":"form","requestedSchema":{"type":"object","properties":{"o":{"type":"string","oneOf":[{"enum":["a","b"]}]}}}}`,
		`{"mode":"form","requestedSchema":{"type":"object","properties":{}}}`,
		`{"mode":"form","requestedSchema":null}`,
		`{"mode":"form"}`,
		`{"mode":"url","url":"https://example.com/auth","message":"go"}`,
		`{"mode":"url","url":"/relative"}`,
		`{"mode":"url","url":"javascript:alert(1)"}`,
		`{"mode":"form","requestId":"stream-1","requestedSchema":{"type":"object","properties":{"a":{"type":"string","default":"d","title":"T","description":"D"}}}}`,
		`{"mode":"form","requestedSchema":{"type":"object","properties":{"a":{"type":"array"}}}}`,
		`{"mode":"form","requestedSchema":{"type":"object","properties":{"a":{"type":"object"}}}}`,
		`{"mode":"sms","message":"q"}`,
		`{}`,
		`null`,
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var wire acpElicitationCreateParams
		if err := json.Unmarshal(data, &wire); err != nil {
			return
		}
		request, err := wireToNeutralElicitation(wire, "codex")
		if err != nil {
			return
		}
		// Invariants the rest of the pipeline relies on.
		if request.Mode != middleware.ElicitationModeForm && request.Mode != middleware.ElicitationModeURL {
			t.Fatalf("projection produced an unusable mode %q", request.Mode)
		}
		if request.Mode == middleware.ElicitationModeForm && len(request.Fields) == 0 {
			t.Fatal("a form must carry at least one field")
		}
		if request.Mode == middleware.ElicitationModeURL {
			lowered := strings.ToLower(request.URL)
			if !strings.HasPrefix(lowered, "http://") && !strings.HasPrefix(lowered, "https://") {
				t.Fatalf("url mode accepted a non-web url %q", request.URL)
			}
		}
		for _, field := range request.Fields {
			switch field.Type {
			case "string", "number", "boolean", "enum":
			default:
				t.Fatalf("projection produced an unsupported field type %q", field.Type)
			}
			if field.Type == "enum" && len(field.Options) == 0 {
				t.Fatalf("enum field %q has no options", field.Name)
			}
			if field.Name == "" {
				t.Fatal("projection produced a field without a name")
			}
		}
		// The projected request must be re-encodable and acceptable to the
		// shared validator with an empty answer set (no panic, no false accept
		// of required fields).
		if _, err := json.Marshal(request); err != nil {
			t.Fatalf("projected request is not marshalable: %v", err)
		}
		if err := middleware.ValidateElicitationValues(request, map[string]interface{}{}); err == nil {
			for _, field := range request.Fields {
				if field.Required {
					t.Fatalf("empty answers passed validation with required field %q", field.Name)
				}
			}
		}
	})
}

// FuzzValidateElicitationValues asserts the shared validator never panics on the
// arbitrary JSON values a frontend can hand it, and that an accepted answer
// round-trips through JSON (the agent must be able to encode it).
func FuzzValidateElicitationValues(f *testing.F) {
	request := middleware.ElicitationRequest{
		Mode: middleware.ElicitationModeForm,
		Fields: []middleware.ElicitationField{
			{Name: "s", Type: "string"},
			{Name: "n", Type: "number"},
			{Name: "b", Type: "boolean"},
			{Name: "e", Type: "enum", Options: []middleware.ElicitationOption{{Value: "x"}, {Value: "y"}}},
		},
	}
	f.Add([]byte(`{"s":"x","n":1,"b":true,"e":"x"}`))
	f.Add([]byte(`{"n":1e308}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var values map[string]interface{}
		if err := json.Unmarshal(data, &values); err != nil {
			return
		}
		if err := middleware.ValidateElicitationValues(request, values); err != nil {
			return
		}
		outcome := middleware.AcceptElicitation(values)
		if _, err := json.Marshal(outcome); err != nil {
			t.Fatalf("accepted answer is not encodable for the agent: %v", err)
		}
	})
}

// FuzzFlexibleRequestID checks the tolerant request-id decoder: it accepts both
// a string and a number and must reject everything else without collapsing two
// distinct ids into one.
func FuzzFlexibleRequestID(f *testing.F) {
	for _, seed := range []string{`"a"`, `"1"`, `1`, `1.5`, `null`, `true`, `{}`, `[]`, `""`, `"  "`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var wire struct {
			RequestID flexibleRequestID `json:"requestId"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return
		}
		id := wire.RequestID.String()
		// The value is only ever used as a scope/log key; it must not contain
		// characters that would corrupt the scope key format.
		if strings.ContainsAny(id, "\x00\n") {
			t.Fatalf("request id carries a control character: %q", id)
		}
		// A requestId that is present and not null must survive decoding. An
		// object without the key is a different case and may decode to empty.
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(data, &envelope); err != nil {
			return
		}
		raw, present := envelope["requestId"]
		if !present || string(raw) == "null" {
			return
		}
		var text string
		if err := json.Unmarshal(raw, &text); err == nil && text == "" {
			return
		}
		if id == "" {
			t.Fatalf("requestId %s decoded to an empty value", raw)
		}
	})
}
