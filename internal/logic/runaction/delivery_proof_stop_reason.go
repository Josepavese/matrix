package runaction

import "github.com/Josepavese/matrix/internal/logic/runtrace"

// OnTurnStopReason keeps the wrapped notifier's optional stop-reason capability
// reachable through the delivery-proof decorator, for the same reason
// OnModelSelection is forwarded beside this file: the decorator adds delivery
// metadata to thought updates, and it must not decide which evidence the run
// trace can receive.
//
// Dropping this capability is not a graceful degradation. The wrapped notifier
// is the only channel a turn has to report what its provider said ended the
// turn, so a decorator that hides it makes the run record "unreported" for a
// reason the provider did state — an absence Matrix would be asserting about a
// peer that was never silent.
func (n *attachProofNotifier) OnTurnStopReason(stopReason string) {
	if n.base == nil {
		return
	}
	if reporter, ok := n.base.(runtrace.TurnStopReasonReporter); ok {
		reporter.OnTurnStopReason(stopReason)
	}
}
