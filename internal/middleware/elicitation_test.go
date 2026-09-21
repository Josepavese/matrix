package middleware

import "testing"

func TestValidateElicitationValues(t *testing.T) {
	req := ElicitationRequest{
		Mode: ElicitationModeForm,
		Fields: []ElicitationField{
			{Name: "strategy", Type: "enum", Required: true, Options: []ElicitationOption{
				{Value: "conservative", Label: "Play safe"},
				{Value: "balanced"},
			}},
			{Name: "retries", Type: "number"},
			{Name: "dry_run", Type: "boolean"},
			{Name: "note", Type: "string"},
		},
	}
	valid := map[string]interface{}{"strategy": "balanced", "retries": float64(3), "dry_run": true, "note": "ok"}
	if err := ValidateElicitationValues(req, valid); err != nil {
		t.Fatalf("valid values rejected: %v", err)
	}
	// Optional fields may be omitted.
	if err := ValidateElicitationValues(req, map[string]interface{}{"strategy": "balanced"}); err != nil {
		t.Fatalf("optional omission rejected: %v", err)
	}
	cases := map[string]map[string]interface{}{
		"missing required": {"retries": float64(1)},
		"unknown field":    {"strategy": "balanced", "sneaky": "x"},
		"bad enum":         {"strategy": "yolo"},
		"wrong number":     {"strategy": "balanced", "retries": "three"},
		"wrong boolean":    {"strategy": "balanced", "dry_run": "yes"},
	}
	for name, values := range cases {
		if err := ValidateElicitationValues(req, values); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func TestValidateElicitationValuesIgnoresURLMode(t *testing.T) {
	if err := ValidateElicitationValues(ElicitationRequest{Mode: ElicitationModeURL}, map[string]interface{}{"anything": "x"}); err != nil {
		t.Fatalf("url mode carries no values and must validate: %v", err)
	}
}
