package middleware

import (
	"errors"
	"fmt"
	"testing"
)

// TestIsConversationTurnActiveRecognisesOnlyItsSentinel keeps the classifier
// honest in both directions: it reports the condition it names, and it does not
// report an unrelated failure as that condition.
//
// The caller never sees the bare sentinel. The one place that produces it - the
// ACP prompt guard - wraps it around the session it refused, and the one place
// that reads it - the live context route in the session manager - classifies a
// route error that has already been through that wrapping. The bare sentinel is
// asserted as well, but the wrapped case is the shape that travels: it is what
// falls if the classifier stops asking errors.Is and starts comparing identity.
func TestIsConversationTurnActiveRecognisesOnlyItsSentinel(t *testing.T) {
	if !IsConversationTurnActive(ErrConversationTurnActive) {
		t.Fatal("the sentinel error must be reported as an active conversation turn")
	}
	wrapped := fmt.Errorf("%w: remote session %s has an active prompt turn", ErrConversationTurnActive, "session-1")
	if !IsConversationTurnActive(wrapped) {
		t.Fatal("the sentinel wrapped around the session it refused must still be reported")
	}
	if IsConversationTurnActive(errors.New("provider closed the stream")) {
		t.Fatal("an unrelated failure must not be reported as an active conversation turn")
	}
}
