package runtrace

import "strings"

// Stop reason vocabulary.
//
// A stop reason is what the peer said ended the turn. Matrix never substitutes
// the value it would have liked to hear: a provider that reported nothing, or
// reported nothing Matrix can read, leaves the field empty and the trace says
// so. The previous behaviour filled the gap with "end_turn", which made a turn
// the provider never concluded indistinguishable from one it concluded
// normally — the exact confusion recorded for the empty runs.
const (
	// StopReasonReportedNone marks a terminal run whose peer reported no stop
	// reason at all.
	StopReasonReportedNone = "unreported"
	// StopReasonReportedUnknown marks a terminal run whose peer reported a value
	// Matrix does not recognise, which is not the same as "end_turn" either.
	StopReasonReportedUnknown = "unknown"
)

// StopReasonMetadataKey is the field a stop reason travels under, so the layer
// that observes one and the layer that stores it agree on a single name.
const StopReasonMetadataKey = "stop_reason"

// KindTurnStopReason is the event a turn records when a provider stated why it
// ended. It is a fact about the turn, kept next to the turn it belongs to, so a
// terminal transition can report what the provider said instead of a value of
// Matrix's own.
const KindTurnStopReason = "agent.turn.stop_reason"

// TurnStopReasonReporter is how a turn reports what the provider said ended it.
// It is an optional capability of the notifier a turn already carries, so the
// layer that observes a stop reason can record it without this layer learning
// anything about the protocol that carried it. A notifier that does not
// implement it records nothing, and the run reports the reason as unreported
// rather than inventing one.
type TurnStopReasonReporter interface {
	OnTurnStopReason(stopReason string)
}

// terminalOutcome is the verdict written for a terminal transition: the status
// the run actually reached and why.
type terminalOutcome struct {
	Status     string
	StopReason string
	Error      string
}

// NormalizeStopReason records what a peer reported without inventing anything.
// An empty report becomes the explicit "nothing was reported" value, and an
// unrecognised report is preserved so an operator can read the peer's own word
// rather than a guess of ours.
func NormalizeStopReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return StopReasonReportedNone
	}
	return reason
}

// IsReportedStopReason reports whether a stop reason is something the peer
// stated, as opposed to Matrix's own marker for a silent peer.
func IsReportedStopReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case "", StopReasonReportedNone, StopReasonReportedUnknown:
		return false
	default:
		return true
	}
}

// completionFailureCode is the failure code written when a run reaches its
// terminal transition carrying no evidence of a turn at all.
const completionFailureCode = "run_no_turn_evidence"

// turnEvidence reports whether a run produced any structured trace of a turn:
// output content, or an event the protocol recorded as part of the turn itself.
//
// The rule is deliberately structural and closed. It reads only event kinds and
// the run's own output field — never event text, never a provider's metadata —
// so a peer that answers in prose and a peer that answers with tool calls are
// both "work observed", and a run whose only events are Matrix's own bookkeeping
// (routing, model selection, prompt sent, session lifecycle) is "no work
// observed". Kinds under the agent.message namespace count by prefix so a new
// message kind cannot silently turn a real answer into a failure.
func turnEvidence(output string, events []Event) bool {
	if strings.TrimSpace(output) != "" {
		return true
	}
	for _, event := range events {
		kind := strings.TrimSpace(event.Kind)
		switch {
		case strings.HasPrefix(kind, "agent.message."):
			return true
		case kind == "tool.call.requested", kind == "tool.result.received":
			return true
		}
	}
	return false
}

// materializeCompletion is the single place that decides what a successful
// provider turn actually means for the run record. A peer that produced a
// message, a tool call or a final answer completed the run; a peer that
// produced nothing at all did not, and the run is failed with a code that says
// so instead of being reported as a success.
func materializeCompletion(output, stopReason string, events []Event) terminalOutcome {
	stopReason = NormalizeStopReason(firstNonEmptyReason(stopReason, observedStopReason(events)))
	if !turnEvidence(output, events) {
		return terminalOutcome{
			Status:     StatusFailed,
			StopReason: stopReason,
			Error:      "provider turn ended with no output, no tool call and no message; the run produced nothing a consumer can use",
		}
	}
	return terminalOutcome{Status: StatusCompleted, StopReason: stopReason}
}

// observedStopReason reads the reason a turn recorded for itself. The value is
// something a provider said; it is never derived from Matrix's own bookkeeping.
func observedStopReason(events []Event) string {
	for _, event := range events {
		if event.Kind != KindTurnStopReason {
			continue
		}
		if reason, ok := event.Metadata[StopReasonMetadataKey].(string); ok {
			return reason
		}
	}
	return ""
}

// firstNonEmptyReason prefers the reason the caller supplied.
func firstNonEmptyReason(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// applyTerminalOutcome writes one terminal verdict onto a run record.
func applyTerminalOutcome(run Run, outcome terminalOutcome) Run {
	run.Status = outcome.Status
	run.StopReason = outcome.StopReason
	run.Error = outcome.Error
	return run
}
