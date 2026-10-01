package runactivity

import (
	"context"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

type turnStopReasonProbe struct {
	reasons []string
}

func (*turnStopReasonProbe) OnThought(middleware.ThoughtUpdate) {}
func (*turnStopReasonProbe) SetHeader(string, string)           {}
func (*turnStopReasonProbe) FormattedHeader() string            { return "" }

func (p *turnStopReasonProbe) OnTurnStopReason(stopReason string) {
	p.reasons = append(p.reasons, stopReason)
}

// The probe is declared as the canonical capability, not merely as a type that
// happens to have the method. The watchdog forwards through a method set it
// declares itself, so if the canonical interface ever grows a method this line
// stops compiling and whoever adds it is sent here, to the decorator that would
// otherwise have started dropping it silently.
var _ runtrace.TurnStopReasonReporter = (*turnStopReasonProbe)(nil)

// TestActivityWatchdogKeepsTurnStopReasonReachable pins the decorator rule for
// the stop reason: a notifier wrapper must not silently drop an optional
// capability. The wrapped notifier is the only channel a turn has to report what
// its provider said ended the turn, so a watchdog that hides it does not degrade
// gracefully — it makes the run record "unreported" for a reason the provider
// did state, which is an absence Matrix asserts about a peer that was not silent.
//
// This is the case that was broken: with an activity timeout configured the
// reason never reached the run record, while the same run without a timeout
// recorded it correctly.
func TestActivityWatchdogKeepsTurnStopReasonReachable(t *testing.T) {
	probe := &turnStopReasonProbe{}
	_, wrapped, _, stop := WithTimeout(context.Background(), time.Minute, probe)
	defer stop()

	reporter, ok := wrapped.(runtrace.TurnStopReasonReporter)
	if !ok {
		t.Fatal("activity watchdog dropped the TurnStopReasonReporter capability")
	}
	reporter.OnTurnStopReason("max_tokens")
	if len(probe.reasons) != 1 || probe.reasons[0] != "max_tokens" {
		t.Fatalf("stop reason did not reach the wrapped notifier unaltered: %#v", probe.reasons)
	}
}

// TestActivityWatchdogAcceptsNotifiersWithoutTurnStopReason keeps the forward a
// no-op for notifiers that publish no stop reason, and proves the wrapper stays
// usable rather than panicking on a notifier that lacks the capability.
func TestActivityWatchdogAcceptsNotifiersWithoutTurnStopReason(t *testing.T) {
	plain := &plainThoughtNotifier{}
	_, wrapped, _, stop := WithTimeout(context.Background(), time.Minute, plain)
	defer stop()

	wrapped.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeThinking, Content: "work"})
	if plain.thoughts != 1 {
		t.Fatalf("thought updates must keep flowing through the wrapper: %d", plain.thoughts)
	}
	if _, ok := wrapped.(runtrace.TurnStopReasonReporter); !ok {
		t.Fatal("the wrapper must always expose the optional capability, even when the inner notifier ignores it")
	}
	reporter, ok := wrapped.(runtrace.TurnStopReasonReporter)
	if !ok {
		t.Fatal("the wrapper must always expose the optional capability, even when the inner notifier ignores it")
	}
	reporter.OnTurnStopReason("end_turn")
}

// TestActivityWatchdogForwardsAnAbsentStopReasonUnchanged proves the decorator
// reports what it was given and nothing more. A wrapper that filled a missing
// reason with a value of its own would replace one invention with another: the
// point of the capability is that the run record states the provider's word, or
// states that there was none.
func TestActivityWatchdogForwardsAnAbsentStopReasonUnchanged(t *testing.T) {
	probe := &turnStopReasonProbe{}
	_, wrapped, _, stop := WithTimeout(context.Background(), time.Minute, probe)
	defer stop()

	reporter, ok := wrapped.(runtrace.TurnStopReasonReporter)
	if !ok {
		t.Fatal("activity watchdog dropped the TurnStopReasonReporter capability")
	}
	reporter.OnTurnStopReason("")
	if len(probe.reasons) != 1 || probe.reasons[0] != "" {
		t.Fatalf("the decorator altered the reported reason: %#v", probe.reasons)
	}
}
