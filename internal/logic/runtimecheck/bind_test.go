package runtimecheck

import (
	"strings"
	"testing"
)

func TestRequireAPIKeyForExternalBind(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		apiKey  string
		wantErr bool
	}{
		{name: "ipv4 loopback without key", addr: "127.0.0.1:9090"},
		{name: "localhost without key", addr: "localhost:9090"},
		{name: "ipv6 loopback without key", addr: "[::1]:9090"},
		{name: "wildcard without key", addr: ":9090", wantErr: true},
		{name: "zero addr without key", addr: "0.0.0.0:9090", wantErr: true},
		{name: "external without key", addr: "192.0.2.10:9090", wantErr: true},
		{name: "external with key", addr: "192.0.2.10:9090", apiKey: "secret"},
		{name: "invalid addr", addr: "not-a-host-port", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireAPIKeyForExternalBind(tt.addr, tt.apiKey, "addr_key", "api_key")
			if (err != nil) != tt.wantErr {
				t.Fatalf("expected err=%v, got %v", tt.wantErr, err)
			}
		})
	}
}

// TestUnauthenticatedLoopbackWarningSaysWhatTheDefaultAccepts: the shipped default is an
// unauthenticated loopback ingress, and the silence about it is the gap. The warning must
// appear exactly when that exposure exists and stay quiet otherwise, so an operator is not
// trained to ignore it.
func TestUnauthenticatedLoopbackWarningSaysWhatTheDefaultAccepts(t *testing.T) {
	cases := []struct {
		name    string
		addr    string
		apiKey  string
		wantGap string
	}{
		{name: "loopback without a key", addr: "127.0.0.1:9091", apiKey: "", wantGap: "matrix_api_key"},
		{name: "non-loopback bind without a key is refused elsewhere, so nothing to warn about", addr: "10.0.0.5:9091", apiKey: "", wantGap: ""},
		{name: "loopback with a key", addr: "127.0.0.1:9091", apiKey: "secret", wantGap: ""},
		{name: "external with a key", addr: "0.0.0.0:9091", apiKey: "secret", wantGap: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := UnauthenticatedLoopbackWarning(tc.addr, tc.apiKey, "matrix_http_addr", "matrix_api_key")
			if tc.wantGap == "" {
				if got != "" {
					t.Fatalf("a configured key must not warn: %s", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantGap) || !strings.Contains(got, tc.addr) {
				t.Fatalf("warning does not name the gap and the address: %q", got)
			}
		})
	}
}

// TestTheDoctorReportSaysWhenTheIngressAcceptsAnyone: the warning has to reach the place
// an operator looks. A report without the authenticated flag must produce it, and one
// with the key configured must not.
func TestTheDoctorReportSaysWhenTheIngressAcceptsAnyone(t *testing.T) {
	mkReport := func(authenticated bool) map[string]any {
		return map[string]any{
			"vault_exists":              true,
			"jsonrpc_daemon_up":         true,
			"matrix_http_up":            true,
			"a2a_http_up":               true,
			"matrix_http_addr":          "127.0.0.1:9091",
			"matrix_http_authenticated": authenticated,
		}
	}

	var warnings []string
	AppendRuntimeWarnings(mkReport(false), &warnings)
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "matrix_api_key") {
		t.Fatalf("an unauthenticated ingress must be reported: %v", warnings)
	}

	warnings = nil
	AppendRuntimeWarnings(mkReport(true), &warnings)
	if joined := strings.Join(warnings, "\n"); strings.Contains(joined, "matrix_api_key") {
		t.Fatalf("a configured key must silence it: %v", warnings)
	}
}
