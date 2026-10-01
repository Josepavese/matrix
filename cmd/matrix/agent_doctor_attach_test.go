package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentdoctor"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runaction"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

type stubRunAttacher struct {
	store  *runtrace.Store
	attach func(*runtrace.Store, middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error)
}

func (s stubRunAttacher) AttachRunContext(_ context.Context, req middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
	return s.attach(s.store, req)
}

// doctorDeliveryRecord drives one real attach through the writer the runtime
// uses, so the doctor is read against a record the runtime would have written.
// The delivery itself happens in the service's goroutine, so callers wait for
// the state they assert.
func doctorDeliveryRecord(t *testing.T, agentID string, attach func(*runtrace.Store, middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error)) middleware.Storage {
	t.Helper()
	storage := memstore.New()
	store := runtrace.NewStore(storage)
	run, _, err := store.Start(runtrace.Run{
		AgentID:          agentID,
		Protocol:         "acp",
		ChannelID:        "task-doctor-attach",
		ExecutionMode:    runtrace.ExecutionModeAsync,
		LogicalSessionID: "logical-doctor-attach",
		RemoteSessionID:  "remote-doctor-attach",
	})
	if err != nil {
		t.Fatalf("Start run: %v", err)
	}
	attacher := stubRunAttacher{store: store, attach: attach}
	_, resp := runaction.New(store, attacher, nil).Handle(context.Background(), run.ID, runaction.Request{
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

func waitForDoctorAttach(t *testing.T, storage middleware.Storage, agentID string, want func(agentdoctor.AttachContextCapability) bool) agentdoctor.AttachContextCapability {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last agentdoctor.AttachContextCapability
	for time.Now().Before(deadline) {
		report, _ := attachContextOf(storage, agentID, true)
		if want(report) {
			return report
		}
		last = report
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the doctor never reported the expected attach context, last = %+v", last)
	return agentdoctor.AttachContextCapability{}
}

func TestDoctorAttachContextReadsTheDeliveryRecordThatWasWritten(t *testing.T) {
	storage := doctorDeliveryRecord(t, "codex", func(*runtrace.Store, middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		return middleware.RunContextAttachmentResult{Status: "delivered", Message: "agent saw marker", LiveConsumptionProven: true}, nil
	})

	report := waitForDoctorAttach(t, storage, "codex", func(report agentdoctor.AttachContextCapability) bool {
		return report.Delivery == agentdoctor.AttachDeliveryProven
	})
	if report.Transport != agentdoctor.AttachTransportAvailable {
		t.Fatalf("transport = %s, want available", report.Transport)
	}
	if report.Provider != agentdoctor.AttachProviderAccepted {
		t.Fatalf("provider = %s, want accepted", report.Provider)
	}
	if !report.Promised {
		t.Fatalf("a proven delivery is the only thing that may promise live context: %+v", report)
	}
}

func TestDoctorAttachContextReadsATypedRefusalAsUnsupported(t *testing.T) {
	storage := doctorDeliveryRecord(t, "opencode", func(*runtrace.Store, middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		return middleware.RunContextAttachmentResult{Unsupported: true, Message: "provider takes no live context"}, nil
	})

	report := waitForDoctorAttach(t, storage, "opencode", func(report agentdoctor.AttachContextCapability) bool {
		return report.Provider == agentdoctor.AttachProviderUnsupported
	})
	if report.Delivery != agentdoctor.AttachDeliveryUnobserved || report.Promised {
		t.Fatalf("a refusal is an answer about the provider, not a delivery: %+v", report)
	}
}

// TestDoctorAttachContextRefusesToPromiseOnAcceptanceAlone is the distinction
// the three levels exist for: the provider took the context, nothing shows it
// arrived, and the doctor must not turn that into a promise.
func TestDoctorAttachContextRefusesToPromiseOnAcceptanceAlone(t *testing.T) {
	storage := doctorDeliveryRecord(t, "codex", func(store *runtrace.Store, req middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		// The run ends before the provider answers, so the recorded delivery is
		// the unproven one and the test does not wait out the boundary window.
		if _, err := store.Complete(req.RunID, "final", "end_turn"); err != nil {
			return middleware.RunContextAttachmentResult{}, err
		}
		return middleware.RunContextAttachmentResult{Status: "delivered", Message: "accepted, unobserved"}, nil
	})

	report := waitForDoctorAttach(t, storage, "codex", func(report agentdoctor.AttachContextCapability) bool {
		return report.Provider == agentdoctor.AttachProviderAccepted
	})
	if report.Delivery != agentdoctor.AttachDeliveryUnproven {
		t.Fatalf("delivery = %s, want unproven without consumption proof", report.Delivery)
	}
	if report.Promised {
		t.Fatalf("acceptance alone must never promise live context: %+v", report)
	}
}

// TestDoctorAttachContextStaysUnobservedWithoutARecord pins the case that was
// never observed: the doctor runs no attach, so an agent nothing was recorded
// for has an unknown capability rather than a credited one.
func TestDoctorAttachContextStaysUnobservedWithoutARecord(t *testing.T) {
	report, note := attachContextOf(memstore.New(), "codex", true)
	if report.Provider != agentdoctor.AttachProviderUnobserved || report.Delivery != agentdoctor.AttachDeliveryUnobserved {
		t.Fatalf("an agent with no record must stay unobserved: %+v", report)
	}
	if report.Promised {
		t.Fatalf("nothing was observed, so nothing may be promised: %+v", report)
	}
	if note != "" {
		t.Fatalf("no record is not a reason to warn, got %q", note)
	}
}

// TestDoctorAttachContextNamesARecordThatHasNotAnsweredYet covers the record the
// attach path writes before the provider answers: it proves an attempt, not a
// capability, and the note says which record was read.
func TestDoctorAttachContextNamesARecordThatHasNotAnsweredYet(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	storage := doctorDeliveryRecord(t, "codex", func(*runtrace.Store, middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		<-release
		return middleware.RunContextAttachmentResult{Status: "delivered", LiveConsumptionProven: true}, nil
	})

	report, note := attachContextOf(storage, "codex", true)
	if report.Provider != agentdoctor.AttachProviderUnobserved || report.Promised {
		t.Fatalf("a pending attach answers nothing about capability: %+v", report)
	}
	if !strings.Contains(note, "pending") {
		t.Fatalf("the note must name the record that was read, got %q", note)
	}
}

// TestDoctorAttachContextRefusesToReadAFailedAttachAsAcceptance covers the other
// record that holds no answer: the attach itself failed, so the capability is
// unknown and the doctor says which class it read instead of crediting the
// provider with an acceptance that never happened.
func TestDoctorAttachContextRefusesToReadAFailedAttachAsAcceptance(t *testing.T) {
	storage := doctorDeliveryRecord(t, "codex", func(*runtrace.Store, middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		return middleware.RunContextAttachmentResult{}, errors.New("provider died during attach")
	})

	// The pending record answers the first read, so the wait is on the failed
	// class itself: reading once would assert against the wrong record.
	var report agentdoctor.AttachContextCapability
	deadline := time.Now().Add(5 * time.Second)
	for {
		var note string
		report, note = attachContextOf(storage, "codex", true)
		if strings.Contains(note, "provider_failed") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the doctor never read the failed delivery, last note %q", note)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if report.Provider != agentdoctor.AttachProviderUnobserved || report.Promised {
		t.Fatalf("a failed attach promises nothing: %+v", report)
	}
}
