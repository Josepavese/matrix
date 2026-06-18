package runtimecheck

import "testing"

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
