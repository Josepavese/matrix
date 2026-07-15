package agentdoctor

import "testing"

func TestDeprecatedCodexACPSourceDetectsLegacyPackage(t *testing.T) {
	got := DeprecatedCodexACPSource("npx", []string{"-y", "@zed-industries/codex-acp@0.16.0"}, "")
	if got == "" {
		t.Fatal("expected deprecated provider source")
	}
}

func TestDeprecatedCodexACPSourceAcceptsCanonicalPackage(t *testing.T) {
	got := DeprecatedCodexACPSource("npx", []string{"-y", "@agentclientprotocol/codex-acp@1.1.2"}, "https://github.com/agentclientprotocol/codex-acp")
	if got != "" {
		t.Fatalf("unexpected deprecated source %q", got)
	}
}
