package runtrace

import (
	"encoding/json"
	"strings"
	"testing"
)

// questionSentinel is the text a person was shown. It is deliberately
// recognisable: the proofs below are negative - the question must not survive a
// policy that keeps content out - and a negative needs something it would have
// found.
const questionSentinel = "Quale database deve usare la migrazione?"

func elicitationOpened(session string, seconds int) Notification {
	return Notification{
		Kind: KindElicitationOpened, RunID: "run-stall", ElicitationID: "el-1",
		SessionID: session, Timestamp: at(seconds),
		Question: questionSentinel, QuestionTruncated: true,
	}
}

func questionInStallView(view StallView) string {
	for _, request := range view.Pending {
		if request.Question != "" {
			return request.Question
		}
	}
	for _, session := range view.Sessions {
		for _, request := range session.Pending {
			if request.Question != "" {
				return request.Question
			}
		}
	}
	return ""
}

func questionTruncatedInStallView(view StallView) bool {
	for _, request := range view.Pending {
		if request.QuestionTruncated {
			return true
		}
	}
	for _, session := range view.Sessions {
		for _, request := range session.Pending {
			if request.QuestionTruncated {
				return true
			}
		}
	}
	return false
}

// TestTheStallViewCarriesTheQuestionBeforeThePolicy fixes where the redaction
// happens: the derivation still holds the text, because the view is built from
// the raw records so it can see who is blocked. What must not hold it is the
// projection that answers a consumer.
func TestTheStallViewCarriesTheQuestionBeforeThePolicy(t *testing.T) {
	view := ObserveStall(stalledRun(StatusRunning), nil, []Notification{elicitationOpened("ses-parent", 1)})
	if got := questionInStallView(view); got != questionSentinel {
		t.Fatalf("la vista grezza porta %q, vuole la domanda della persona", got)
	}
	if !questionTruncatedInStallView(view) {
		t.Fatal("la vista grezza ha perso il marker di troncamento")
	}
}

// TestTheElicitationQuestionDoesNotSurviveARedactingPolicy is the constraint on
// the field: the question is user content, so any policy that keeps content out
// of the trace removes it from the projected view and from the marshalled trace
// a consumer receives.
func TestTheElicitationQuestionDoesNotSurviveARedactingPolicy(t *testing.T) {
	for _, mode := range []string{ContentModeRefs, ContentModeRedacted} {
		t.Run(mode, func(t *testing.T) {
			run := stalledRun(StatusRunning)
			run.TracePolicy = TracePolicy{ContentMode: mode}
			trace := Project(run, nil, []Notification{elicitationOpened("ses-parent", 1)})
			if got := questionInStallView(*trace.Stall); got != "" {
				t.Fatalf("content_mode=%s proietta ancora la domanda: %q", mode, got)
			}
			if questionTruncatedInStallView(*trace.Stall) {
				t.Fatalf("content_mode=%s conserva il marker di un testo che non c'è più", mode)
			}
			encoded, err := json.Marshal(trace)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), questionSentinel) {
				t.Fatalf("content_mode=%s esporta il testo della domanda: %s", mode, encoded)
			}
		})
	}
}

// TestTheElicitationQuestionSurvivesOnlyWhereThePolicyKeepsContent is the other
// half: the field is not dead, it reaches the consumer that opted into content,
// and the truncation marker travels with it.
func TestTheElicitationQuestionSurvivesOnlyWhereThePolicyKeepsContent(t *testing.T) {
	run := stalledRun(StatusRunning)
	run.TracePolicy = TracePolicy{ContentMode: ContentModeInline}
	trace := Project(run, nil, []Notification{elicitationOpened("ses-parent", 1)})
	if got := questionInStallView(*trace.Stall); got != questionSentinel {
		t.Fatalf("content_mode=inline perde la domanda: %q", got)
	}
	if !questionTruncatedInStallView(*trace.Stall) {
		t.Fatal("content_mode=inline perde il marker di troncamento")
	}
}
