package runaction

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentdoctor"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

// recordedAttach drives one real attach through the service, so the record under
// test is the one the attach path writes rather than a copy of its shape typed
// out here. The delivery itself happens in the service's own goroutine, which is
// why the tests below wait for the state they assert instead of reading once.
func recordedAttach(t *testing.T, agentID string, attach func(context.Context, middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error)) middleware.Storage {
	t.Helper()
	storage := memstore.New()
	store := runtrace.NewStore(storage)
	run, _, err := store.Start(runtrace.Run{
		AgentID:          agentID,
		Protocol:         "acp",
		ChannelID:        "task-delivery-observation",
		ExecutionMode:    runtrace.ExecutionModeAsync,
		LogicalSessionID: "logical-delivery-observation",
		RemoteSessionID:  "remote-delivery-observation",
	})
	if err != nil {
		t.Fatalf("Start run: %v", err)
	}
	_, resp := New(store, fakeAttacher{attach: attach}, nil).Handle(context.Background(), run.ID, Request{
		Action: "attach_context",
		SidecarCapsules: []middleware.SidecarCapsule{
			{Provider: "noema", ID: "ctx", Visibility: middleware.SidecarVisibilityLLMVisible, Content: "marker"},
		},
	})
	if !resp.Accepted {
		t.Fatalf("attach not accepted: %+v", resp)
	}
	return storage
}

func waitForObservation(t *testing.T, storage middleware.Storage, agentID string, want func(agentdoctor.AttachObservation) bool) agentdoctor.AttachObservation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last agentdoctor.AttachObservation
	for time.Now().Before(deadline) {
		observation, err := LatestAttachObservation(storage, agentID)
		if err != nil {
			t.Fatalf("LatestAttachObservation: %v", err)
		}
		if want(observation) {
			return observation
		}
		last = observation
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the recorded observation never reached the expected state, last = %+v", last)
	return agentdoctor.AttachObservation{}
}

// TestLatestAttachObservationReadsWhatTheAttachPathWrote pins the reading side
// against the writing side: the keys, the class and the proof flag are the ones
// the attach path actually wrote, so a change to the record reaches this test.
func TestLatestAttachObservationReadsWhatTheAttachPathWrote(t *testing.T) {
	storage := recordedAttach(t, "codex", func(context.Context, middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		return middleware.RunContextAttachmentResult{Status: "delivered", Message: "agent saw marker", LiveConsumptionProven: true}, nil
	})

	observation := waitForObservation(t, storage, "codex", func(observation agentdoctor.AttachObservation) bool {
		return observation.LiveConsumptionProven
	})
	if !observation.Attempted {
		t.Fatalf("a proven delivery must be reported as an attempt: %+v", observation)
	}
	if observation.DeliveryClass != deliveryClassLiveActivityObserved {
		t.Fatalf("delivery class = %q, want the class the attach path recorded", observation.DeliveryClass)
	}
}

func TestLatestAttachObservationReadsATypedRefusalAsUnsupported(t *testing.T) {
	storage := recordedAttach(t, "opencode", func(context.Context, middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		return middleware.RunContextAttachmentResult{Unsupported: true, Message: "provider takes no live context"}, nil
	})

	observation := waitForObservation(t, storage, "opencode", func(observation agentdoctor.AttachObservation) bool {
		return observation.Unsupported
	})
	if !observation.Attempted || observation.LiveConsumptionProven {
		t.Fatalf("a refusal is an answer, not a delivery: %+v", observation)
	}
	if observation.DeliveryClass != deliveryClassUnsupported {
		t.Fatalf("delivery class = %q, want the recorded refusal", observation.DeliveryClass)
	}
}

// TestLatestAttachObservationStaysUnknownWithoutARecord pins the case that was
// never observed: an agent with no delivery record has an unknown capability,
// and a run belonging to another agent is not its record.
func TestLatestAttachObservationStaysUnknownWithoutARecord(t *testing.T) {
	empty := memstore.New()
	observation, err := LatestAttachObservation(empty, "codex")
	if err != nil {
		t.Fatalf("LatestAttachObservation on an empty store: %v", err)
	}
	if observation.Attempted || observation.Unsupported || observation.LiveConsumptionProven || observation.DeliveryClass != "" {
		t.Fatalf("an agent with no record must stay unobserved, got %+v", observation)
	}

	storage := recordedAttach(t, "opencode", func(context.Context, middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		return middleware.RunContextAttachmentResult{Status: "delivered", LiveConsumptionProven: true}, nil
	})
	waitForObservation(t, storage, "opencode", func(observation agentdoctor.AttachObservation) bool {
		return observation.LiveConsumptionProven
	})
	elsewhere, err := LatestAttachObservation(storage, "codex")
	if err != nil {
		t.Fatalf("LatestAttachObservation for another agent: %v", err)
	}
	if elsewhere.Attempted || elsewhere.DeliveryClass != "" {
		t.Fatalf("another agent's delivery was read as this agent's: %+v", elsewhere)
	}
}

// TestLatestAttachObservationRefusesToInferCapabilityFromAnUnansweredDelivery
// covers the recorded statuses that are not an answer from the provider. The
// metadata keys are pinned by the writer-driven tests above; this table pins the
// mapping, including a failed attach, which must not read as an acceptance.
func TestLatestAttachObservationRefusesToInferCapabilityFromAnUnansweredDelivery(t *testing.T) {
	for _, test := range []struct {
		why           string
		metadata      map[string]interface{}
		wantAttempted bool
		wantClass     string
	}{
		{why: "pending", metadata: map[string]interface{}{"delivery_status": "accepted", "delivery_class": "pending"}, wantClass: "pending"},
		{why: "failed", metadata: map[string]interface{}{"delivery_status": "failed", "delivery_class": "provider_failed"}, wantClass: "provider_failed"},
		{why: "no status at all", metadata: map[string]interface{}{}},
		{
			why:           "returned without proof",
			metadata:      map[string]interface{}{"delivery_status": "unverified", "delivery_class": "provider_returned_unverified"},
			wantAttempted: true,
			wantClass:     "provider_returned_unverified",
		},
	} {
		t.Run(test.why, func(t *testing.T) {
			observation := observationOfAttachEvent(runtrace.Event{Kind: attachEventKind, Metadata: test.metadata})
			if observation.Attempted != test.wantAttempted {
				t.Fatalf("attempted = %v, want %v for %+v", observation.Attempted, test.wantAttempted, test.metadata)
			}
			if observation.LiveConsumptionProven {
				t.Fatalf("a delivery with no proof must not be reported as proven: %+v", observation)
			}
			if observation.DeliveryClass != test.wantClass {
				t.Fatalf("delivery class = %q, want %q", observation.DeliveryClass, test.wantClass)
			}
		})
	}
}

// TestLatestAttachObservationFailsLoudlyWhenARecordCannotBeRead keeps a broken
// store from reporting an unknown capability as if nothing had happened: the
// caller has to be able to say why the levels are unknown.
func TestLatestAttachObservationFailsLoudlyWhenARecordCannotBeRead(t *testing.T) {
	storage := &unreadableStorage{Storage: memstore.New()}
	if _, err := LatestAttachObservation(storage, "codex"); err == nil {
		t.Fatal("an unreadable run listing must be reported, not silently read as no record")
	}
}

type unreadableStorage struct{ middleware.Storage }

func (s *unreadableStorage) List(string) ([]string, error) {
	return nil, errors.New("run trace storage is unreadable")
}

// TestRunKeyWithNoRunIDIsThePrefixStoredRunsStartWith guards the property the
// accessor relies on to find an agent's runs: without it, every agent would be
// reported as unknown while the code still compiled and ran.
func TestRunKeyWithNoRunIDIsThePrefixStoredRunsStartWith(t *testing.T) {
	if !strings.HasPrefix(runtrace.RunKey("run-1"), runtrace.RunKey("")) {
		t.Fatalf("RunKey(%q) does not start with RunKey(%q)", "run-1", "")
	}
}
