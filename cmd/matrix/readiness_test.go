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

func TestEvaluateReadinessIgnoresOptionalRuntimeDownWarnings(t *testing.T) {
	report := evaluateReadiness(
		map[string]any{
			"vault_exists": true,
			"warnings": []any{
				"jsonrpc daemon is not reachable on 127.0.0.1:9090",
				"acp http server is not reachable on 127.0.0.1:9091",
				"a2a http server is not reachable on 127.0.0.1:9091",
			},
		},
		map[string]any{},
		map[string]any{"schema": map[string]any{"status": "current"}},
		map[string]any{"encryption": map[string]any{"configured": true, "plaintext_keys": 0}},
		false,
	)
	if report["status"] != "ready" {
		t.Fatalf("expected ready, got %+v", report)
	}
}

func TestEvaluateReadinessKeepsRuntimeDownWarningsWhenExpected(t *testing.T) {
	report := evaluateReadiness(
		map[string]any{
			"vault_exists":      true,
			"jsonrpc_daemon_up": false,
			"warnings": []string{
				"jsonrpc daemon is not reachable on 127.0.0.1:9090",
			},
		},
		map[string]any{},
		map[string]any{"schema": map[string]any{"status": "current"}},
		map[string]any{"encryption": map[string]any{"configured": true, "plaintext_keys": 0}},
		true,
	)
	if report["status"] != "not_ready" {
		t.Fatalf("expected not_ready, got %+v", report)
	}
}
