package runaction

import (
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

type deliveryModelProbe struct {
	selections []middleware.ModelSelection
}

func (*deliveryModelProbe) OnThought(middleware.ThoughtUpdate) {}
func (*deliveryModelProbe) SetHeader(string, string)           {}
func (*deliveryModelProbe) FormattedHeader() string            { return "" }

func (p *deliveryModelProbe) OnModelSelection(selection middleware.ModelSelection) {
	p.selections = append(p.selections, selection)
}

type plainDeliveryNotifier struct{ thoughts int }

func (n *plainDeliveryNotifier) OnThought(middleware.ThoughtUpdate) { n.thoughts++ }
func (*plainDeliveryNotifier) SetHeader(string, string)             {}
func (*plainDeliveryNotifier) FormattedHeader() string              { return "" }

// TestDeliveryProofNotifierKeepsModelSelectionReachable pins the same decorator
// rule as the activity watchdog: wrapping a notifier to add delivery proof must
// not remove the optional capability that carries model attestation to the run
// trace.
func TestDeliveryProofNotifierKeepsModelSelectionReachable(t *testing.T) {
	probe := &deliveryModelProbe{}
	wrapped := newAttachProofNotifier(probe, "delivery-1")

	forwarder, ok := interface{}(wrapped).(middleware.ModelSelectionNotifier)
	if !ok {
		t.Fatal("delivery proof wrapper dropped the ModelSelectionNotifier capability")
	}
	forwarder.OnModelSelection(middleware.ModelSelection{
		ConfiguredModel: "chosen-model", VerificationReason: middleware.ModelUnverifiedProviderDoesNotAttest,
	})
	if len(probe.selections) != 1 || probe.selections[0].ConfiguredModel != "chosen-model" {
		t.Fatalf("model selection did not reach the wrapped notifier: %#v", probe.selections)
	}
}

// TestDeliveryProofNotifierAcceptsNotifiersWithoutModelSelection keeps the
// forward a no-op for notifiers that publish no model evidence.
func TestDeliveryProofNotifierAcceptsNotifiersWithoutModelSelection(t *testing.T) {
	wrapped := newAttachProofNotifier(&plainDeliveryNotifier{}, "delivery-2")
	if _, ok := interface{}(wrapped).(middleware.ModelSelectionNotifier); !ok {
		t.Fatal("the wrapper must always expose the optional capability")
	}
	wrapped.OnModelSelection(middleware.ModelSelection{ConfiguredModel: "chosen-model"})
}
