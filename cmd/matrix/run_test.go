package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthorizeMatrixRuntimeRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/_matrix/runtime", nil)
	w := httptest.NewRecorder()
	if authorizeMatrixRuntimeRequest(w, req, "secret") {
		t.Fatal("expected runtime request without API key to be rejected")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/_matrix/runtime", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	if !authorizeMatrixRuntimeRequest(w, req, "secret") {
		t.Fatal("expected bearer-authenticated runtime request to be accepted")
	}
}

func TestElicitTimeoutFallsBackToServiceDefault(t *testing.T) {
	cases := map[string]struct {
		value string
		want  time.Duration
	}{
		"configured seconds": {"300", 300 * time.Second},
		"missing key":        {"0", 0},
		"non-numeric":        {"soon", 0},
		"negative":           {"-5", 0},
	}
	for name, tc := range cases {
		got := elicitTimeout(func(key, fallback string) string {
			if key != "agent.elicitation_timeout_seconds" {
				t.Fatalf("%s: unexpected key %q", name, key)
			}
			if fallback != "0" {
				t.Fatalf("%s: expected zero fallback, got %q", name, fallback)
			}
			return tc.value
		})
		if got != tc.want {
			t.Fatalf("%s: expected %v, got %v", name, tc.want, got)
		}
	}
}

func TestElicitationIsOptInByDefault(t *testing.T) {
	get := func(key, fallback string) string {
		if key != "agent.elicitation_enabled" {
			t.Fatalf("unexpected key %q", key)
		}
		if fallback != "false" {
			t.Fatalf("elicitation must default to disabled, got fallback %q", fallback)
		}
		return fallback
	}
	if elicitationEnabled(get) {
		t.Fatal("elicitation must stay disabled unless the operator opts in")
	}
	for _, value := range []string{"true", "TRUE", " true "} {
		if !elicitationEnabled(func(string, string) string { return value }) {
			t.Fatalf("value %q must enable elicitation", value)
		}
	}
	for _, value := range []string{"false", "", "yes", "1", "enabled"} {
		if elicitationEnabled(func(string, string) string { return value }) {
			t.Fatalf("value %q must not enable elicitation", value)
		}
	}
}
