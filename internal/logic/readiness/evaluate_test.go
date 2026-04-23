package readiness

import "testing"

func TestEvaluateStatus(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want string
	}{
		{
			name: "schema outdated blocks readiness",
			in: Input{
				RuntimeReport: map[string]any{"vault_exists": true},
				StorageReport: map[string]any{"schema": map[string]any{
					"status": "outdated",
				}},
				VaultReport: map[string]any{"encryption": map[string]any{"configured": true}},
			},
			want: "not_ready",
		},
		{
			name: "warnings keep ready_with_warnings",
			in: Input{
				RuntimeReport: map[string]any{"vault_exists": true, "warnings": []any{"runtime warning"}},
				StorageReport: map[string]any{
					"schema": map[string]any{"status": "current"},
					"workspaces": []any{
						map[string]any{"id": "billing-api", "timeline_prunable": true},
					},
				},
				VaultReport: map[string]any{"encryption": map[string]any{"configured": false}},
			},
			want: "ready_with_warnings",
		},
		{
			name: "optional runtime down warnings ignored",
			in: Input{
				RuntimeReport: map[string]any{
					"vault_exists": true,
					"warnings": []any{
						"jsonrpc daemon is not reachable on 127.0.0.1:9090",
						"matrix http ingress is not reachable on 127.0.0.1:9091",
						"a2a http server is not reachable on 127.0.0.1:9091",
					},
				},
				StorageReport: map[string]any{"schema": map[string]any{"status": "current"}},
				VaultReport:   map[string]any{"encryption": map[string]any{"configured": true, "plaintext_keys": 0}},
			},
			want: "ready",
		},
		{
			name: "expected runtime down blocks",
			in: Input{
				RuntimeReport: map[string]any{
					"vault_exists":      true,
					"jsonrpc_daemon_up": false,
					"warnings":          []string{"jsonrpc daemon is not reachable on 127.0.0.1:9090"},
				},
				StorageReport:   map[string]any{"schema": map[string]any{"status": "current"}},
				VaultReport:     map[string]any{"encryption": map[string]any{"configured": true, "plaintext_keys": 0}},
				ExpectRuntimeUp: true,
			},
			want: "not_ready",
		},
		{
			name: "runtime-owned vault lock accepted",
			in: Input{
				RuntimeReport: map[string]any{
					"vault_exists":      true,
					"jsonrpc_daemon_up": true,
					"matrix_http_up":    true,
				},
				LoggingReport: map[string]any{"warnings": []string{
					"logging config unavailable, using fallback defaults: vault error: [ERR_VAULT_OPEN] Failed to open bbolt database: timeout (op: bolt.NewProvider)",
				}},
				StorageReport: map[string]any{"schema": map[string]any{
					"status": "unavailable",
					"error":  "[ERR_VAULT_OPEN] Failed to open bbolt database: timeout (op: bolt.NewProvider)",
				}},
				VaultReport:     map[string]any{"encryption": map[string]any{"configured": true, "plaintext_keys": 0}},
				ExpectRuntimeUp: true,
			},
			want: "ready",
		},
		{
			name: "vault lock still blocks when runtime down",
			in: Input{
				RuntimeReport: map[string]any{
					"vault_exists":      true,
					"jsonrpc_daemon_up": false,
					"matrix_http_up":    false,
				},
				StorageReport: map[string]any{"schema": map[string]any{
					"status": "unavailable",
					"error":  "[ERR_VAULT_OPEN] Failed to open bbolt database: timeout (op: bolt.NewProvider)",
				}},
				VaultReport:     map[string]any{"encryption": map[string]any{"configured": true, "plaintext_keys": 0}},
				ExpectRuntimeUp: true,
			},
			want: "not_ready",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Evaluate(tc.in)["status"]; got != tc.want {
				t.Fatalf("expected %s, got %+v", tc.want, got)
			}
		})
	}
}
