package sessioncleanup

import (
	"errors"
	"testing"

	"github.com/jose/matrix-v2/internal/middleware"
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

func TestEphemeralCleanupRequiresStrongProof(t *testing.T) {
	input := CleanInput{
		Ephemeral:              true,
		RemoteSessionID:        "remote-1",
		CleanupPolicy:          middleware.SessionCleanupPolicyForgetLocal,
		ProcessReapRequired:    true,
		ProcessRetentionReason: NoMatchingCachedAgentClient,
		LocalForgotten:         true,
	}
	if IsClean(input) {
		t.Fatalf("ephemeral local-only cleanup must not be clean without provider or process proof")
	}
	if got := Strength(input); got != StrengthFailed {
		t.Fatalf("unexpected strength: %q", got)
	}
	if got := WeakReason(input); got != WeakCleanupNoRemoteOrProcessProof {
		t.Fatalf("unexpected weak reason: %q", got)
	}
}

func TestRemoteCancelIsStrongCleanupProof(t *testing.T) {
	input := CleanInput{
		Ephemeral:       true,
		RemoteSessionID: "remote-1",
		CleanupPolicy:   middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
		RemoteCanceled:  true,
		LocalForgotten:  true,
	}
	if !IsClean(input) {
		t.Fatalf("remote cancel should be clean")
	}
	if !HasStrongProof(input) {
		t.Fatalf("remote cancel should be strong proof")
	}
	if got := Strength(input); got != StrengthStrong {
		t.Fatalf("unexpected strength: %q", got)
	}
}

func TestProcessReapIsStrongCleanupProofForEphemeralRemoteSession(t *testing.T) {
	input := CleanInput{
		Ephemeral:           true,
		RemoteSessionID:     "remote-1",
		CleanupPolicy:       middleware.SessionCleanupPolicyDeleteRemoteOrCancelAndForgetLocal,
		ProcessReapRequired: true,
		ProcessReaped:       true,
		LocalForgotten:      true,
	}
	if !IsClean(input) {
		t.Fatalf("process reap should satisfy ephemeral remote cleanup")
	}
	if !HasStrongProof(input) {
		t.Fatalf("process reap should be strong proof")
	}
	if got := Strength(input); got != StrengthStrong {
		t.Fatalf("unexpected strength: %q", got)
	}
}

func TestRetainedProcessIsExplicitWeakCleanup(t *testing.T) {
	input := CleanInput{
		RemoteSessionID:         "remote-1",
		CleanupPolicy:           middleware.SessionCleanupPolicyForgetLocal,
		ProcessRetained:         true,
		ProcessRetentionAllowed: true,
		ProcessRetentionReason:  OtherLocalSessionsStillReferenceAgentClient,
		LocalForgotten:          true,
	}
	if !IsClean(input) {
		t.Fatalf("retained non-ephemeral cleanup should remain operationally clean")
	}
	if HasStrongProof(input) {
		t.Fatalf("retention is not strong cleanup proof")
	}
	if got := Strength(input); got != StrengthRetained {
		t.Fatalf("unexpected strength: %q", got)
	}
	if got := WeakReason(input); got != WeakCleanupProcessRetained {
		t.Fatalf("unexpected weak reason: %q", got)
	}
}
