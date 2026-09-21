package middleware

import (
	"encoding/json"
	"math"
	"testing"
)

// ----------------------------------------------------------------------------
// Adversarial validation suite
// ----------------------------------------------------------------------------
//
// ValidateElicitationValues is the single gate every frontend shares, so its
// edge behaviour is a correctness boundary, not a detail: what it accepts here
// is what reaches a real agent.

func formWith(fields ...ElicitationField) ElicitationRequest {
	return ElicitationRequest{Mode: ElicitationModeForm, Fields: fields}
}

func TestValidateRejectsNonFiniteNumbers(t *testing.T) {
	request := formWith(ElicitationField{Name: "n", Type: "number", Required: true})
	for name, value := range map[string]interface{}{
		"NaN":     math.NaN(),
		"+Inf":    math.Inf(1),
		"-Inf":    math.Inf(-1),
		"string":  "42",
		"bool":    true,
		"null":    nil,
		"object":  map[string]interface{}{"a": 1},
		"array":   []interface{}{1},
		"jsonNum": json.Number("oops"),
	} {
		if err := ValidateElicitationValues(request, map[string]interface{}{"n": value}); err == nil {
			t.Fatalf("%s must be rejected as a number", name)
		}
	}
	for name, value := range map[string]interface{}{
		"float": float64(1.5),
		"int":   3,
		"int64": int64(7),
		"zero":  float64(0),
	} {
		if err := ValidateElicitationValues(request, map[string]interface{}{"n": value}); err != nil {
			t.Fatalf("%s must be accepted as a number: %v", name, err)
		}
	}
}

// TestValidateRejectsNilValuesForRequiredFields keeps a nil entry from
// satisfying a required field by accident.
func TestValidateRejectsNilValuesForRequiredFields(t *testing.T) {
	request := formWith(ElicitationField{Name: "a", Type: "string", Required: true})
	if err := ValidateElicitationValues(request, map[string]interface{}{"a": nil}); err == nil {
		t.Fatal("an explicit nil must not satisfy a required field")
	}
	optional := formWith(ElicitationField{Name: "a", Type: "string"})
	if err := ValidateElicitationValues(optional, map[string]interface{}{"a": nil}); err != nil {
		t.Fatalf("nil for an optional field must be tolerated: %v", err)
	}
}

// TestValidateEnumUsesOptionsNotLegacyEnum guards the single representation: a
// field built with Options must enforce exactly those values.
func TestValidateEnumUsesOptionsNotLegacyEnum(t *testing.T) {
	request := formWith(ElicitationField{
		Name: "scope", Type: "enum", Required: true,
		Options: []ElicitationOption{{Value: "once"}, {Value: "always"}},
	})
	if err := ValidateElicitationValues(request, map[string]interface{}{"scope": "always"}); err != nil {
		t.Fatalf("declared option rejected: %v", err)
	}
	if err := ValidateElicitationValues(request, map[string]interface{}{"scope": "sometimes"}); err == nil {
		t.Fatal("undeclared option accepted")
	}
	// An enum field with no options can never be satisfied.
	empty := formWith(ElicitationField{Name: "scope", Type: "enum", Required: true})
	if err := ValidateElicitationValues(empty, map[string]interface{}{"scope": "once"}); err == nil {
		t.Fatal("an enum without options must reject every value")
	}
}

// TestValidateUnknownTypeIsPermissive documents the boundary: the projection
// rejects unknown types before they reach validation, so a hand-built request
// with an exotic type must not silently accept garbage values.
func TestValidateUnknownTypeIsPermissive(t *testing.T) {
	request := formWith(ElicitationField{Name: "x", Type: "mystery", Required: true})
	if err := ValidateElicitationValues(request, map[string]interface{}{"x": "anything"}); err != nil {
		t.Fatalf("unreachable type must not block validation: %v", err)
	}
}

// TestValidateRejectsUnknownFieldsEvenWhenEmpty keeps strictness symmetric.
func TestValidateRejectsUnknownFieldsEvenWhenEmpty(t *testing.T) {
	request := formWith(ElicitationField{Name: "a", Type: "string"})
	if err := ValidateElicitationValues(request, map[string]interface{}{"a": "v", "b": ""}); err == nil {
		t.Fatal("an unknown field must be rejected even when its value is empty")
	}
	if err := ValidateElicitationValues(request, nil); err != nil {
		t.Fatalf("no values at all must be acceptable for an optional field: %v", err)
	}
}

// TestValidateURLModeIgnoresValuesEverywhere pins the mode contract.
func TestValidateURLModeIgnoresValuesEverywhere(t *testing.T) {
	request := ElicitationRequest{Mode: ElicitationModeURL, URL: "https://x.example"}
	if err := ValidateElicitationValues(request, map[string]interface{}{"anything": 1}); err != nil {
		t.Fatalf("url mode carries no values: %v", err)
	}
}

// TestElicitationOptionHelpersAreTotal keeps the helpers safe on degenerate
// values; a panic here would take down a channel frontend.
func TestElicitationOptionHelpersAreTotal(t *testing.T) {
	var zero ElicitationField
	if values := zero.OptionValues(); len(values) != 0 {
		t.Fatalf("zero field must yield no options: %v", values)
	}
	field := ElicitationField{Options: []ElicitationOption{{Value: "a"}, {Value: "b"}}}
	values := field.OptionValues()
	if len(values) != 2 || values[0] != "a" || values[1] != "b" {
		t.Fatalf("unexpected option values: %v", values)
	}
}

// TestOutcomeConstructorsProduceValidActions keeps every constructor inside the
// three actions the protocol admits.
func TestOutcomeConstructorsProduceValidActions(t *testing.T) {
	valid := map[string]bool{
		ElicitationActionAccept: true, ElicitationActionDecline: true, ElicitationActionCancel: true,
	}
	for name, outcome := range map[string]ElicitationOutcome{
		"accept":  AcceptElicitation(map[string]interface{}{"a": 1}),
		"decline": DeclineElicitation(),
		"cancel":  CancelElicitation(),
	} {
		if !valid[outcome.Action] {
			t.Fatalf("%s produced an invalid action %q", name, outcome.Action)
		}
	}
	if AcceptElicitation(map[string]interface{}{}).Action != ElicitationActionAccept {
		t.Fatal("accept with empty values must stay accept")
	}
}
