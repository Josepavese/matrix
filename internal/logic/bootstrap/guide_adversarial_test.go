package bootstrap

import (
	"strings"
	"testing"
)

// TestBuildGuideNamesWhatIsMissing is the onboarding contract: the guide must
// tell an operator which of the three preconditions is unmet, because a guide
// that says "configure Matrix" is not actionable.
func TestBuildGuideNamesWhatIsMissing(t *testing.T) {
	empty := strings.Join(BuildGuide("127.0.0.1:9091", false, false, false, nil), "\n")
	for _, want := range []string{"matrix agent enable", "Telegram is optional", "onboarding is not complete"} {
		if !strings.Contains(empty, want) {
			t.Fatalf("the guide for a fresh install must mention %q, got:\n%s", want, empty)
		}
	}

	// A channel that is enabled but not configured is the trap that leaves the
	// gateway silently dead, so it must be called out separately.
	halfConfigured := strings.Join(BuildGuide("127.0.0.1:9091", true, true, false, []string{"codex"}), "\n")
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
	done := strings.Join(BuildGuide("127.0.0.1:9091", true, true, true, []string{"codex"}), "\n")
	if strings.Contains(done, "onboarding is not complete") {
		t.Fatalf("a completed setup must not be told to onboard, got:\n%s", done)
	}
}

// TestTheGuideNamesTheAddressTheRuntimeWillBind: the step that tells an operator where
// to POST used to name 127.0.0.1:9091 unconditionally, so anyone who moved the port was
// sent to an address with nothing listening - found by doing exactly that. The step
// must name the configured address, and fall back to the default only when none is set.
func TestTheGuideNamesTheAddressTheRuntimeWillBind(t *testing.T) {
	moved := strings.Join(BuildGuide("127.0.0.1:9291", true, false, false, []string{"codex"}), "\n")
	if !strings.Contains(moved, "127.0.0.1:9291/v1/runs") {
		t.Fatalf("the guide must name the configured address:\n%s", moved)
	}
	if strings.Contains(moved, "9091/v1/runs") {
		t.Fatalf("the guide must not name the default port as well:\n%s", moved)
	}

	fallback := strings.Join(BuildGuide("", true, false, false, []string{"codex"}), "\n")
	if !strings.Contains(fallback, "127.0.0.1:9091/v1/runs") {
		t.Fatalf("an unset address must fall back to the shipped default:\n%s", fallback)
	}
}
