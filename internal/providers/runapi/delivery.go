package runapi

import (
	"context"
	"time"

	"github.com/Josepavese/matrix/internal/logic/deliverycontract"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

// declaredContract validates the caller's contract before any run exists. A
// contract that cannot be evaluated is refused here rather than recorded and
// discovered later: an accepted run carrying an uncheckable contract would
// produce exactly the formal acceptance this feature exists to prevent.
//
// A contract that declares nothing is not a contract. It returns nil so the run
// proceeds with no declaration at all, rather than with an empty one that a
// reader could mistake for a satisfied requirement list.
func (req runRequest) declaredContract() (*deliverycontract.Contract, error) {
	if req.DeliveryContract == nil {
		return nil, nil
	}
	if err := req.DeliveryContract.Validate(); err != nil {
		return nil, err
	}
	if !req.DeliveryContract.Declared() {
		return nil, nil
	}
	return req.DeliveryContract, nil
}

// appendDeliveryDeclared records the caller's delivery contract before the run
// starts, so that what was asked for is on the record before anything can be
// said about whether it arrived.
func appendDeliveryDeclared(store *runtrace.Store, runID string, contract deliverycontract.Contract) {
	if !contract.Declared() {
		return
	}
	_, _ = store.AppendEvent(runtrace.Event{
		RunID: runID, Kind: deliverycontract.EventDeclared, Actor: "matrix",
		Status: runtrace.StatusRunning, Timestamp: time.Now().UTC(),
		Metadata: deliverycontract.Encode(contract),
	})
}

// recordDeliveryVerdict evaluates the declared contract once, when the run
// reaches a terminal state, and records the answer.
//
// Once is the whole point. A verdict recomputed on every read would change under
// the reader's feet the moment somebody touched the workspace after the run, and
// then the record would no longer say what was true when the work stopped.
func (s *Server) recordDeliveryVerdict(run runtrace.Run) {
	events, err := s.runStore.LoadEvents(run.ID, 0)
	if err != nil {
		return
	}
	contract, decided, declared := deliveryFromEvents(events)
	if !declared || decided {
		return
	}
	verdict := deliverycontract.Evaluate(context.Background(), run.WorkspacePath, contract)
	_, _ = s.runStore.AppendEvent(runtrace.Event{
		RunID: run.ID, Kind: deliverycontract.EventVerified, Actor: "matrix",
		Status: verdict.Status, Timestamp: time.Now().UTC(),
		Metadata: deliverycontract.Encode(verdict),
	})
}

// deliveryFromEvents reads back what a run declared and whether it was decided.
// The third result distinguishes "no contract was declared" from an empty one,
// because the first must never be reported as a judgement about delivery.
func deliveryFromEvents(events []runtrace.Event) (deliverycontract.Contract, bool, bool) {
	var contract deliverycontract.Contract
	declared, decided := false, false
	for _, event := range events {
		switch event.Kind {
		case deliverycontract.EventDeclared:
			declared = deliverycontract.Decode(event.Metadata, &contract) || declared
		case deliverycontract.EventVerified:
			decided = true
		}
	}
	return contract, decided, declared
}

// deliveryExplanation is what the diagnostic surfaces report about delivery. A
// declared contract the run never got to evaluate is reported as unverifiable
// with that reason, rather than as a missing verdict the reader has to interpret.
func deliveryExplanation(events []runtrace.Event) (string, *deliverycontract.Verdict) {
	_, decided, declared := deliveryFromEvents(events)
	if !declared {
		return deliverycontract.StatusNotDeclared, nil
	}
	if !decided {
		return deliverycontract.StatusUnverifiable, &deliverycontract.Verdict{
			Status: deliverycontract.StatusUnverifiable,
			Reason: "the run ended before its declared delivery contract could be evaluated",
		}
	}
	for _, event := range events {
		if event.Kind != deliverycontract.EventVerified {
			continue
		}
		var verdict deliverycontract.Verdict
		if deliverycontract.Decode(event.Metadata, &verdict) {
			return verdict.Status, &verdict
		}
		// The payload was redacted or unreadable, but the event's own status
		// survived: report what is still known instead of losing the verdict.
		return event.Status, &deliverycontract.Verdict{Status: event.Status}
	}
	return deliverycontract.StatusUnverifiable, nil
}
