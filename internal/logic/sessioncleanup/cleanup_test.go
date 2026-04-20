package sessioncleanup

import (
	"errors"
	"testing"
)

func TestFailureCodeDetectsAgentStartContextCancelledDuringCleanup(t *testing.T) {
	err := errors.New("failed to start agent /tmp/codex-acp: context canceled")
	if got := FailureCode(err); got != FailureAgentStartContextCancelledDuringCleanup {
		t.Fatalf("unexpected failure code: %q", got)
	}
}

func TestFailureCodeIgnoresUnrelatedErrors(t *testing.T) {
	if got := FailureCode(errors.New("session/delete unsupported")); got != "" {
		t.Fatalf("unexpected failure code: %q", got)
	}
}
