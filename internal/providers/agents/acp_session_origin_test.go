package agents

import (
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestSessionOriginIsReadFromTheAttachHandshake proves the origin a turn reports
// is the method the provider actually answered, and that anything else stays
// unknown rather than being guessed from a session ID or from provider metadata.
func TestSessionOriginIsReadFromTheAttachHandshake(t *testing.T) {
	for _, tc := range []struct {
		method string
		want   sessionOrigin
	}{
		{method: "session/resume", want: sessionOriginResumed},
		{method: "session/load", want: sessionOriginLoaded},
		{method: " session/load ", want: sessionOriginLoaded},
		{method: "", want: sessionOriginUnknown},
		{method: "session/new", want: sessionOriginUnknown},
		{method: "list", want: sessionOriginUnknown},
	} {
		t.Run(tc.method, func(t *testing.T) {
			if got := originFromVerificationMethod(tc.method); got != tc.want {
				t.Fatalf("origin(%q) = %q, want %q", tc.method, got, tc.want)
			}
		})
	}
}

// TestOriginIsOnlyReportedWhenTheProviderConfirmed proves a transition that did
// not happen reports unknown: an origin is evidence, not an assumption. This is
// what keeps "the provider restated this session's state" from being claimed for
// a session that was never asked.
func TestOriginIsOnlyReportedWhenTheProviderConfirmed(t *testing.T) {
	if got := originIf(true, sessionOriginResumed); got != sessionOriginResumed {
		t.Fatalf("a confirmed resume was reported as %q", got)
	}
	if got := originIf(false, sessionOriginResumed); got != sessionOriginUnknown {
		t.Fatalf("an unconfirmed transition was reported as %q", got)
	}
}

// TestSessionOriginVocabularyIsStable pins the vocabulary a consumer reads. The
// values are part of what a caller sees, so they cannot drift silently.
func TestSessionOriginVocabularyIsStable(t *testing.T) {
	for value, want := range map[sessionOrigin]string{
		sessionOriginUnknown: "",
		sessionOriginCreated: "created",
		sessionOriginResumed: "resumed",
		sessionOriginLoaded:  "loaded",
		sessionOriginReused:  "reused",
	} {
		if string(value) != want {
			t.Fatalf("origin %q changed to %q", want, string(value))
		}
	}
}

// stopReasonSink records what a turn reported, standing in for a run's notifier.
type stopReasonSink struct{ reported []string }

func (s *stopReasonSink) OnThought(middleware.ThoughtUpdate) {}

func (s *stopReasonSink) SetHeader(_, _ string) {}

func (s *stopReasonSink) FormattedHeader() string { return "" }

func (s *stopReasonSink) OnTurnStopReason(stopReason string) {
	s.reported = append(s.reported, stopReason)
}

// TestTurnReportsOnlyWhatTheProviderSaid proves the hand-off to the run record
// carries the peer's word and nothing else. A turn whose response and terminal
// state both carried no reason reports nothing, so the run records "unreported"
// rather than a reason Matrix supplied.
func TestTurnReportsOnlyWhatTheProviderSaid(t *testing.T) {
	for _, tc := range []struct {
		name      string
		response  string
		observed  string
		want      string
		reporting bool
	}{
		{name: "response carries it", response: "end_turn", want: "end_turn", reporting: true},
		{name: "terminal state carries it", observed: "refusal", want: "refusal", reporting: true},
		{name: "response wins", response: "max_tokens", observed: "end_turn", want: "max_tokens", reporting: true},
		{name: "nobody reported one", reporting: false},
		{name: "blank is not a report", response: "   ", observed: " ", reporting: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := &stopReasonSink{}
			turn := middleware.ConversationTurn{ThoughtNotifier: sink}
			reportTurnStopReason(turn, resolveTurnStopReason(tc.response, tc.observed))
			if tc.reporting {
				if len(sink.reported) != 1 || sink.reported[0] != tc.want {
					t.Fatalf("reported %q, want %q", sink.reported, tc.want)
				}
				return
			}
			if len(sink.reported) != 0 {
				t.Fatalf("an absent stop reason was reported as %q", sink.reported)
			}
		})
	}
}
