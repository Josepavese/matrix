package providerdiag

import (
	"strings"
	"testing"
)

func TestSanitizeProviderStderrRedactsSecretsAndBoundsOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	raw := "Error: " + home + "/.codex/config.toml OPENAI_API_KEY=sk-secretvalue123 Bearer tokenvalue123 " + strings.Repeat("x", 600)

	got := Sanitize(raw)

	for _, secret := range []string{"sk-secretvalue123", "tokenvalue123", home + "/.codex"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitized stderr leaked %q: %q", secret, got)
		}
	}
	if !strings.Contains(got, "~/.codex/config.toml") {
		t.Fatalf("sanitized stderr lost useful path: %q", got)
	}
	if len(got) > MaxStderr {
		t.Fatalf("sanitized stderr length = %d", len(got))
	}
}
