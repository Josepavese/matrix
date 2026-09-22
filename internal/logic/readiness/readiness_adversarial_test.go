package readiness

import "testing"

// TestEvaluateBlocksOnAMissingVault is the release-gate behaviour: readiness must
// refuse to say "ready" when the vault is absent, because every later step would
// fail with a confusing error instead.
func TestEvaluateBlocksOnAMissingVault(t *testing.T) {
	report := Evaluate(Input{})
	if report["status"] != "not_ready" {
		t.Fatalf("a missing vault must be not_ready, got %v", report["status"])
	}
	blockers, _ := report["blockers"].([]string)
	if len(blockers) == 0 {
		t.Fatalf("a blocked report must explain why: %v", report)
	}
}

// TestEvaluateRequiresTheDaemonOnlyWhenAsked keeps the default (artifact and
// install) validation from failing on a stopped host, while still blocking when
// the operator explicitly expects a live runtime.
func TestEvaluateRequiresTheDaemonOnlyWhenAsked(t *testing.T) {
	withVault := Input{RuntimeReport: map[string]any{"vault_exists": true}}

	relaxed := Evaluate(withVault)
	if blockers, _ := relaxed["blockers"].([]string); len(blockers) != 0 {
		t.Fatalf("a stopped runtime must not block by default: %v", relaxed)
	}
	if relaxed["status"] == "not_ready" {
		t.Fatalf("unexpected status: %v", relaxed["status"])
	}

	strict := Evaluate(Input{RuntimeReport: map[string]any{"vault_exists": true}, ExpectRuntimeUp: true})
	if strict["status"] != "not_ready" {
		t.Fatalf("an expected-but-absent runtime must be not_ready, got %v", strict["status"])
	}
	found := false
	blockers, ok := strict["blockers"].([]string)
	if !ok {
		t.Fatalf("the report must carry a blocker list: %v", strict)
	}
	for _, blocker := range blockers {
		if blocker == "jsonrpc daemon is not reachable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the blocker must name the daemon, got %v", strict["blockers"])
	}
}
