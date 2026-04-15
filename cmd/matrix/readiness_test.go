package main

import "testing"

func TestEvaluateReadinessNotReadyWhenSchemaOutdated(t *testing.T) {
	report := evaluateReadiness(
		map[string]any{"vault_exists": true},
		map[string]any{},
		map[string]any{"schema": map[string]any{"status": "outdated"}},
		map[string]any{"encryption": map[string]any{"configured": true}},
		false,
	)
	if report["status"] != "not_ready" {
		t.Fatalf("expected not_ready, got %+v", report)
	}
}

func TestEvaluateReadinessReadyWithWarnings(t *testing.T) {
	report := evaluateReadiness(
		map[string]any{"vault_exists": true, "warnings": []any{"runtime warning"}},
		map[string]any{},
		map[string]any{
			"schema": map[string]any{"status": "current"},
			"workspaces": []any{
				map[string]any{"id": "billing-api", "timeline_prunable": true},
			},
		},
		map[string]any{"encryption": map[string]any{"configured": false}},
		false,
	)
	if report["status"] != "ready_with_warnings" {
		t.Fatalf("expected ready_with_warnings, got %+v", report)
	}
}
