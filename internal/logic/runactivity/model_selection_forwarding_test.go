package runactivity

import (
	"context"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

type modelSelectionProbe struct {
	selections []middleware.ModelSelection
}

func (*modelSelectionProbe) OnThought(middleware.ThoughtUpdate) {}
func (*modelSelectionProbe) SetHeader(string, string)           {}
func (*modelSelectionProbe) FormattedHeader() string            { return "" }

func (p *modelSelectionProbe) OnModelSelection(selection middleware.ModelSelection) {
	p.selections = append(p.selections, selection)
}

type plainThoughtNotifier struct{ thoughts int }

func (n *plainThoughtNotifier) OnThought(middleware.ThoughtUpdate) { n.thoughts++ }
func (*plainThoughtNotifier) SetHeader(string, string)             {}
func (*plainThoughtNotifier) FormattedHeader() string              { return "" }

// TestActivityWatchdogKeepsModelSelectionReachable pins the decorator rule: a
// notifier wrapper must not silently drop an optional capability. The ACP
// adapter publishes model attestation only through ModelSelectionNotifier, so a
// watchdog that hides it makes the run trace report an unverified model even
// when the provider confirmed it.
func TestActivityWatchdogKeepsModelSelectionReachable(t *testing.T) {
	probe := &modelSelectionProbe{}
	_, wrapped, _, stop := WithTimeout(context.Background(), time.Minute, probe)
	defer stop()

	forwarder, ok := wrapped.(middleware.ModelSelectionNotifier)
	if !ok {
		t.Fatal("activity watchdog dropped the ModelSelectionNotifier capability of the wrapped notifier")
	}
	forwarder.OnModelSelection(middleware.ModelSelection{
		ConfiguredModel: "chosen-model", Verification: middleware.ModelVerificationUnverified,
		VerificationReason: middleware.ModelUnverifiedNotRepeated,
	})
	if len(probe.selections) != 1 {
		t.Fatalf("model selection did not reach the wrapped notifier: %#v", probe.selections)
	}
	if got := probe.selections[0]; got.ConfiguredModel != "chosen-model" ||
		got.VerificationReason != middleware.ModelUnverifiedNotRepeated {
		t.Fatalf("model selection was altered by the wrapper: %#v", got)
	}
}

// TestActivityWatchdogAcceptsNotifiersWithoutModelSelection keeps the wrapper
// usable for plain notifiers, and proves the forward is a no-op rather than a
// panic when the wrapped implementation has no model evidence to publish.
func TestActivityWatchdogAcceptsNotifiersWithoutModelSelection(t *testing.T) {
	plain := &plainThoughtNotifier{}
	_, wrapped, _, stop := WithTimeout(context.Background(), time.Minute, plain)
	defer stop()

	wrapped.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeThinking, Content: "work"})
	if plain.thoughts != 1 {
		t.Fatalf("thought updates must keep flowing through the wrapper: %d", plain.thoughts)
	}
	if _, ok := wrapped.(middleware.ModelSelectionNotifier); !ok {
		t.Fatal("the wrapper must always expose the optional capability, even when the inner notifier ignores it")
	}
	forwarder, ok := wrapped.(middleware.ModelSelectionNotifier)
	if !ok {
		t.Fatal("the wrapper must always expose the optional capability, even when the inner notifier ignores it")
	}
	forwarder.OnModelSelection(middleware.ModelSelection{ConfiguredModel: "chosen-model"})
}
