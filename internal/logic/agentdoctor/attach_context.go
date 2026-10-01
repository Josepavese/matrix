// Package agentdoctor reports what is known about an agent before a run.
package agentdoctor

// The three levels of "can this agent receive live context", kept as three
// separate answers because collapsing them into one boolean is how "the
// transport could carry it" becomes "the work was delivered". Each level is a
// different kind of claim, made from different evidence:
//
//	transport — the runtime has something that can attach context at all
//	provider  — what happened when an attach was actually attempted
//	delivery  — whether the provider was seen consuming what it accepted
//
// Only the last is evidence that anything arrived. The first is a capability of
// Matrix's own wiring, and the second is a promise the provider made.
const (
	AttachTransportAvailable = "available"
	AttachTransportAbsent    = "absent"

	AttachProviderAccepted    = "accepted"
	AttachProviderUnsupported = "unsupported"
	AttachProviderUnobserved  = "unobserved"

	AttachDeliveryProven     = "proven"
	AttachDeliveryUnproven   = "unproven"
	AttachDeliveryUnobserved = "unobserved"
)

// AttachObservation is what Matrix has actually seen for one agent, taken from
// the delivery records the attach path already writes. Nothing here is inferred
// from the agent's name or configuration.
type AttachObservation struct {
	// Attempted is false until an attach has been tried for this agent.
	Attempted bool
	// Unsupported is the provider's typed refusal. It is not a failure: the
	// provider answered, and the answer was that it does not take live context.
	Unsupported bool
	// LiveConsumptionProven is the positive proof: the provider was observed
	// consuming the attached context during the run.
	LiveConsumptionProven bool
	// ActivityCount is how much live activity was observed, kept so a caller can
	// see the difference between "proven" and "proven by one event".
	ActivityCount int
	// DeliveryClass is the classification recorded with the observation.
	DeliveryClass string
}

// AttachContextCapability is the report. Promised is the single question a
// caller actually has: may I tell a user their live context will arrive? It is
// true only when a delivery was proven, never merely because the provider
// accepted the request.
type AttachContextCapability struct {
	Transport string `json:"transport"`
	Provider  string `json:"provider"`
	Delivery  string `json:"delivery"`
	Promised  bool   `json:"delivery_promised"`
	Reason    string `json:"reason"`
	// TerminalIndependent records the guarantee that does hold regardless of all
	// three levels: the run's terminal result is delivered by Matrix's own path,
	// which never depends on the live attach. A refused attach withholds the
	// live context only, and the doctor says so instead of letting a refusal
	// read as a run whose result will never arrive.
	TerminalIndependent bool `json:"terminal_delivery_independent_of_attach"`
}

// DescribeAttachContext answers the three levels from the transport's own
// availability and what was observed for the agent.
func DescribeAttachContext(transportAvailable bool, observation AttachObservation) AttachContextCapability {
	report := AttachContextCapability{TerminalIndependent: true}
	if !transportAvailable {
		report.Transport = AttachTransportAbsent
		report.Provider = AttachProviderUnobserved
		report.Delivery = AttachDeliveryUnobserved
		report.Reason = "the runtime has nothing that can attach live context, so none will be offered"
		return report
	}
	report.Transport = AttachTransportAvailable
	if !observation.Attempted {
		report.Provider = AttachProviderUnobserved
		report.Delivery = AttachDeliveryUnobserved
		report.Reason = "no attach has been attempted for this agent, so its capability is unknown rather than absent"
		return report
	}
	if observation.Unsupported {
		report.Provider = AttachProviderUnsupported
		report.Delivery = AttachDeliveryUnobserved
		report.Reason = "the provider refused live context; no delivery is promised, and the run's terminal result is unaffected"
		return report
	}
	report.Provider = AttachProviderAccepted
	if observation.LiveConsumptionProven {
		report.Delivery = AttachDeliveryProven
		report.Promised = true
		report.Reason = "the provider accepted live context and was observed consuming it"
		return report
	}
	report.Delivery = AttachDeliveryUnproven
	report.Reason = "the provider accepted live context but was never observed consuming it, so acceptance is all that is known"
	return report
}
