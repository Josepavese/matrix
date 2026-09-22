package bootstrap

import (
	"strings"
	"testing"
)

// TestBuildGuideNamesWhatIsMissing is the onboarding contract: the guide must
// tell an operator which of the three preconditions is unmet, because a guide
// that says "configure Matrix" is not actionable.
func TestBuildGuideNamesWhatIsMissing(t *testing.T) {
	empty := strings.Join(BuildGuide(false, false, false, nil), "\n")
	for _, want := range []string{"matrix agent enable", "Telegram is optional", "onboarding is not complete"} {
		if !strings.Contains(empty, want) {
			t.Fatalf("the guide for a fresh install must mention %q, got:\n%s", want, empty)
		}
	}

	// A channel that is enabled but not configured is the trap that leaves the
	// gateway silently dead, so it must be called out separately.
	halfConfigured := strings.Join(BuildGuide(true, true, false, []string{"codex"}), "\n")
	if !strings.Contains(halfConfigured, "Telegram is enabled but not configured") {
		t.Fatalf("an enabled-but-unconfigured channel must be reported, got:\n%s", halfConfigured)
	}
	if strings.Contains(halfConfigured, "matrix agent enable") {
		t.Fatal("a configured agent must not be reported as missing")
	}
	if !strings.Contains(halfConfigured, "codex") {
		t.Fatal("the active agents must be named")
	}

	// A fully configured system must not be told to onboard again.
	done := strings.Join(BuildGuide(true, true, true, []string{"codex"}), "\n")
	if strings.Contains(done, "onboarding is not complete") {
		t.Fatalf("a completed setup must not be told to onboard, got:\n%s", done)
	}
}
