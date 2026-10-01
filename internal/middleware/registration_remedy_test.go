package middleware

import (
	"strings"
	"testing"
)

// TestRegistrationPendingRemedyIsTheSentenceBothSurfacesPublish pins the value
// itself, not its use: this sentence reaches the operator through the runtime
// status and through the run API's refusal, so it is asserted where it is
// defined. A downstream test would still pass after a rewrite here, because a
// downstream test compares against whatever this package publishes today.
func TestRegistrationPendingRemedyIsTheSentenceBothSurfacesPublish(t *testing.T) {
	const want = "restart the daemon or wait for the next refresh"
	if RegistrationPendingRemedy != want {
		t.Fatalf("RegistrationPendingRemedy = %q, want %q", RegistrationPendingRemedy, want)
	}
}

// TestRegistrationRemedyThenRefusesToComposeADanglingAction keeps the rule from
// degrading into a concatenation: a surface that has no next step must not
// publish "remedy, then " with nothing after it.
func TestRegistrationRemedyThenRefusesToComposeADanglingAction(t *testing.T) {
	for _, action := range []string{"", "   ", "\t", "\n  \t "} {
		got := RegistrationRemedyThen(action)
		if got != RegistrationPendingRemedy {
			t.Fatalf("RegistrationRemedyThen(%q) = %q, want the bare remedy %q", action, got, RegistrationPendingRemedy)
		}
		if strings.Contains(got, ", then") || strings.HasSuffix(got, ",") {
			t.Fatalf("RegistrationRemedyThen(%q) published a dangling join: %q", action, got)
		}
	}
}

// TestRegistrationRemedyThenAppendsTheSurfaceAction fixes the composed form byte
// for byte, spacing included: the surfaces append their own verb and both are
// read by an operator who has to see one instruction, not two variants.
func TestRegistrationRemedyThenAppendsTheSurfaceAction(t *testing.T) {
	for _, test := range []struct{ why, action, want string }{
		{why: "plain action", action: "retry", want: "restart the daemon or wait for the next refresh, then retry"},
		{why: "padded action", action: "  re-run this command\t", want: "restart the daemon or wait for the next refresh, then re-run this command"},
	} {
		t.Run(test.why, func(t *testing.T) {
			if got := RegistrationRemedyThen(test.action); got != test.want {
				t.Fatalf("RegistrationRemedyThen(%q) = %q, want %q", test.action, got, test.want)
			}
		})
	}
}
