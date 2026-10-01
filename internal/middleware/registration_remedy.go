package middleware

import "strings"

// RegistrationPendingRemedy is the recovery step shared by every surface that
// reports a registration the runtime has not applied yet.
//
// An operator meets this sentence in the window between recording a change in
// the SSOT and the daemon picking it up: the runtime status reports such an
// agent as pending_apply, and the run API refuses a request that depends on the
// change it has not applied. Two surfaces wording the same recovery step
// differently would contradict each other about the one thing the message is
// for — what the operator has to do next — so the sentence lives here once, and
// each surface appends the verb of its own next step.
//
// It names no agent, no provider and no program: it is the same step whatever
// the registration was.
const RegistrationPendingRemedy = "restart the daemon or wait for the next refresh"

// RegistrationRemedyThen returns the shared remedy followed by what the surface
// wants the operator to do once the runtime has applied the registration.
func RegistrationRemedyThen(action string) string {
	trimmed := strings.TrimSpace(action)
	if trimmed == "" {
		return RegistrationPendingRemedy
	}
	return RegistrationPendingRemedy + ", then " + trimmed
}
