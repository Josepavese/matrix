package agentdoctor

import "testing"

// TestAProviderWithoutTheCapabilityIsRefusedWithoutPromisingDelivery is the
// PM's first criterion: a provider that does not take live context must come
// back refused, and nothing may be promised about a delivery that will not
// happen.
func TestAProviderWithoutTheCapabilityIsRefusedWithoutPromisingDelivery(t *testing.T) {
	report := DescribeAttachContext(true, AttachObservation{Attempted: true, Unsupported: true})
	if report.Provider != AttachProviderUnsupported {
		t.Fatalf("provider = %q, want %q", report.Provider, AttachProviderUnsupported)
	}
	if report.Promised {
		t.Fatal("a refusal was reported as a promised delivery")
	}
	if report.Reason == "" {
		t.Fatal("the refusal does not say what it means")
	}
}

// TestAcceptanceIsNotDelivery is the distinction the whole feature exists for. A
// provider that accepted the context and was never seen consuming it has given a
// promise, not evidence, and the report must not upgrade it.
func TestAcceptanceIsNotDelivery(t *testing.T) {
	report := DescribeAttachContext(true, AttachObservation{Attempted: true})
	if report.Provider != AttachProviderAccepted {
		t.Fatalf("provider = %q, want %q", report.Provider, AttachProviderAccepted)
	}
	if report.Delivery != AttachDeliveryUnproven {
		t.Fatalf("delivery = %q, want %q", report.Delivery, AttachDeliveryUnproven)
	}
	if report.Promised {
		t.Fatal("acceptance alone was reported as a promised delivery")
	}
}

// TestProvenDeliveryIsTheOnlyThingPromised is the positive half of the PM's
// criterion: the compatible provider is reported from the proof of receipt seen
// during the run, and only then may a delivery be promised.
func TestProvenDeliveryIsTheOnlyThingPromised(t *testing.T) {
	report := DescribeAttachContext(true, AttachObservation{
		Attempted: true, LiveConsumptionProven: true, ActivityCount: 3,
		DeliveryClass: "live_activity_observed",
	})
	if report.Delivery != AttachDeliveryProven || !report.Promised {
		t.Fatalf("report = %#v, want a proven delivery", report)
	}
}

// TestAnUnobservedAgentIsUnknownNotIncapable keeps absence of evidence from
// reading as evidence of absence: an agent nobody has tried to attach for is
// unobserved, which is a different answer from "cannot".
func TestAnUnobservedAgentIsUnknownNotIncapable(t *testing.T) {
	report := DescribeAttachContext(true, AttachObservation{})
	if report.Provider != AttachProviderUnobserved || report.Delivery != AttachDeliveryUnobserved {
		t.Fatalf("report = %#v, want both levels unobserved", report)
	}
	if report.Promised {
		t.Fatal("an unobserved agent was reported as a promised delivery")
	}
	if report.Provider == AttachProviderUnsupported {
		t.Fatal("never having tried was reported as a refusal")
	}
}

// TestNoTransportRefusesWithoutClaimingTheProviderWasAsked keeps Matrix's own
// wiring separate from the provider's behaviour: with nothing to attach through,
// no provider was refused and the report must not pretend one was.
func TestNoTransportRefusesWithoutClaimingTheProviderWasAsked(t *testing.T) {
	report := DescribeAttachContext(false, AttachObservation{Attempted: true, LiveConsumptionProven: true})
	if report.Transport != AttachTransportAbsent {
		t.Fatalf("transport = %q, want %q", report.Transport, AttachTransportAbsent)
	}
	if report.Promised {
		t.Fatal("a delivery was promised with no transport to deliver it")
	}
	if report.Provider == AttachProviderUnsupported {
		t.Fatal("the transport's absence was reported as the provider refusing")
	}
}

// TestTheTerminalDoesNotDependOnTheAttach is the PM's last requirement: whatever
// the three levels say, the run's terminal result travels by Matrix's own path,
// and every report has to say so. Otherwise a refused attach reads as a run
// whose result will never arrive.
func TestTheTerminalDoesNotDependOnTheAttach(t *testing.T) {
	cases := []struct {
		name        string
		transport   bool
		observation AttachObservation
	}{
		{name: "no transport", transport: false},
		{name: "provider refused", transport: true, observation: AttachObservation{Attempted: true, Unsupported: true}},
		{name: "accepted, undelivered", transport: true, observation: AttachObservation{Attempted: true}},
		{name: "proven", transport: true, observation: AttachObservation{Attempted: true, LiveConsumptionProven: true}},
		{name: "unobserved", transport: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if report := DescribeAttachContext(tc.transport, tc.observation); !report.TerminalIndependent {
				t.Fatalf("the report does not state that the run's terminal is delivered regardless of the attach: %#v", report)
			}
		})
	}
}
