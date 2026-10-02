package deliverycontract

import "encoding/json"

// The two events a contracted run emits. The contract is recorded as an event
// rather than as a field of the run for one reason: a declaration made before
// the run must be immutable by construction, and a run field can be rewritten by
// the terminal path that records the outcome.
//
// The verdict's own status is written to the event's Status field and not only
// into the payload, because a trace policy may redact event metadata while the
// status survives it. A reader who sees only "run.delivery.verified, incomplete"
// still learns the thing that matters.
const (
	EventDeclared = "run.delivery.declared"
	EventVerified = "run.delivery.verified"
	// EventValidatorExecuted is written once per validator execution. Caller
	// supplied code runs inside the daemon's own privilege, so every run of it
	// leaves a record naming what ran: the argv as declared and the binary the
	// daemon resolved. The command's output is absent by construction and must
	// stay absent, which is why this event carries facts about the execution
	// rather than its result.
	EventValidatorExecuted = "run.delivery.validator.executed"
)

// MetadataKey is where the payload sits inside an event's metadata.
const MetadataKey = "delivery"

// Encode renders a contract or a verdict for an event's metadata. It returns nil
// rather than a partial payload when encoding fails: an undecodable payload must
// read as absent, never as something a reader could mistake for a decision.
func Encode(payload interface{}) map[string]interface{} {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return map[string]interface{}{MetadataKey: string(encoded)}
}

// Decode reads a payload back out of event metadata. It reports whether the
// metadata carried a decodable payload, so a caller can always distinguish
// "nothing was declared" from "something was declared and could not be read".
func Decode(metadata map[string]interface{}, target interface{}) bool {
	if len(metadata) == 0 {
		return false
	}
	raw, ok := metadata[MetadataKey]
	if !ok {
		return false
	}
	text, ok := raw.(string)
	if !ok || text == "" {
		return false
	}
	return json.Unmarshal([]byte(text), target) == nil
}
