package bootstrap

import (
	"strings"
	"testing"
)

func TestBuildGuideTreatsRunsAsNonInteractive(t *testing.T) {
	guide := strings.Join(BuildGuide(false, false, false, []string{"opencode"}), "\n")
	if strings.Contains(guide, "or `/v1/runs`") {
		t.Fatalf("bootstrap guide must not suggest /v1/runs for first-run onboarding: %s", guide)
	}
	if !strings.Contains(guide, "interactive channel") {
		t.Fatalf("bootstrap guide should direct setup through an interactive channel: %s", guide)
	}
}
