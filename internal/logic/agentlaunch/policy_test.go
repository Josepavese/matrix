package agentlaunch

import "testing"

func TestMetadataDetectsCodexTrustedTerminalConfigArgs(t *testing.T) {
	meta := Metadata([]string{"-c", "sandbox_mode=\"danger-full-access\"", "-c", "approval_policy=\"never\""})
	if meta["trusted_terminal"] != true {
		t.Fatalf("expected trusted terminal evidence, got %+v", meta)
	}
	if meta["sandbox_mode"] != "danger-full-access" || meta["approval_policy"] != "never" {
		t.Fatalf("expected codex launch policy, got %+v", meta)
	}
}

func TestMetadataDetectsCodexBypassFlag(t *testing.T) {
	meta := Metadata([]string{"--dangerously-bypass-approvals-and-sandbox"})
	if meta["bypass_approvals_and_sandbox"] != true || meta["trusted_terminal"] != true {
		t.Fatalf("expected trusted bypass evidence, got %+v", meta)
	}
}

func TestMetadataIgnoresUnknownArgs(t *testing.T) {
	if meta := Metadata([]string{"--model", "gpt-5"}); meta != nil {
		t.Fatalf("expected nil metadata for unknown args, got %+v", meta)
	}
}
