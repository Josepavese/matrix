package logredact

import (
	"strings"
	"testing"
)

// TestEndpointIsRedactedByDefault pins the protective default: a provider host
// never reaches a log unless the operator asked for it.
func TestEndpointIsRedactedByDefault(t *testing.T) {
	t.Setenv(revealEndpointsEnv, "")
	for _, raw := range []string{
		"api.deepseek.com",
		"https://api.deepseek.com/v1",
		"http://user:password@api.deepseek.com",
		"127.0.0.1:8080",
	} {
		if got := Endpoint(raw); got != Redacted {
			t.Fatalf("Endpoint(%q) = %q, want %q", raw, got, Redacted)
		}
	}
	if RevealEndpoints() {
		t.Fatal("the zero configuration must not reveal endpoints")
	}
}

// TestEndpointRevealsOnlyOnTheDocumentedOptIn keeps the exception narrow: one
// documented value, and nothing that merely looks like a truthy string.
func TestEndpointRevealsOnlyOnTheDocumentedOptIn(t *testing.T) {
	for _, tc := range []struct {
		value  string
		reveal bool
	}{
		{value: "1", reveal: true},
		{value: " 1 ", reveal: true},
		{value: "", reveal: false},
		{value: "0", reveal: false},
		{value: "true", reveal: false},
		{value: "yes", reveal: false},
		{value: "TRUE", reveal: false},
	} {
		t.Setenv(revealEndpointsEnv, tc.value)
		if got := RevealEndpoints(); got != tc.reveal {
			t.Fatalf("RevealEndpoints() with %q = %v, want %v", tc.value, got, tc.reveal)
		}
		if got := Endpoint("api.deepseek.com"); (got == "api.deepseek.com") != tc.reveal {
			t.Fatalf("Endpoint with %q = %q, want revealed=%v", tc.value, got, tc.reveal)
		}
	}
}

// TestEmptyEndpointStaysEmpty keeps the placeholder honest: it marks a value
// that was withheld, not a target that does not exist.
func TestEmptyEndpointStaysEmpty(t *testing.T) {
	t.Setenv(revealEndpointsEnv, "")
	for _, raw := range []string{"", "   ", "\t"} {
		if got := Endpoint(raw); got != "" {
			t.Fatalf("Endpoint(%q) = %q, want an empty result", raw, got)
		}
	}
}

// TestHeaderNamesNeverCarryValues is the second half of the policy: a caller can
// record which headers were configured without writing a single credential.
func TestHeaderNamesNeverCarryValues(t *testing.T) {
	const secret = "sk-live-should-never-be-written"
	names := HeaderNames(map[string]string{
		"Authorization": "Bearer " + secret,
		"X-Api-Key":     secret,
		"":              secret,
		"  ":            secret,
	})
	if len(names) != 2 || names[0] != "Authorization" || names[1] != "X-Api-Key" {
		t.Fatalf("header names = %#v", names)
	}
	for _, name := range names {
		if strings.Contains(name, secret) {
			t.Fatalf("a header value reached a name: %q", name)
		}
	}
	if names := HeaderNames(nil); len(names) != 0 {
		t.Fatalf("no headers must produce no names: %#v", names)
	}
}
