package runaction

import (
	"context"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

type fakeAttacher struct {
	attach func(context.Context, middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error)
}

func (f fakeAttacher) AttachRunContext(ctx context.Context, req middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
	return f.attach(ctx, req)
}

func TestAttachContextMarksLateWhenRunCompletesBeforeDeliveryReturns(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{
		AgentID:          "opencode",
		Protocol:         "acp",
		ChannelID:        "noema.http",
		ExecutionMode:    runtrace.ExecutionModeAsync,
		LogicalSessionID: "logical-live",
		RemoteSessionID:  "remote-live",
		TracePolicy:      runtrace.TracePolicy{ContentMode: runtrace.ContentModeInline},
	})
	if err != nil {
		t.Fatalf("Start run: %v", err)
	}
	attacher := fakeAttacher{attach: func(_ context.Context, req middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		if _, err := store.Complete(req.RunID, "final", "end_turn"); err != nil {
			t.Fatalf("Complete run: %v", err)
		}
		return middleware.RunContextAttachmentResult{Status: "delivered", Message: "agent saw marker"}, nil
	}}
	_, resp := New(store, attacher, nil).Handle(context.Background(), run.ID, Request{
		Action: "attach_context",
		SidecarCapsules: []middleware.SidecarCapsule{
			{Provider: "noema", ID: "ctx", Visibility: middleware.SidecarVisibilityLLMVisible, Content: "marker"},
		},
	})
	if !resp.Accepted {
		t.Fatalf("expected accepted response, got %+v", resp)
	}
	event := waitRunActionEvent(t, store, run.ID, "run.context.attached", "late")
	if event.Message != "agent saw marker" {
		t.Fatalf("expected late delivery message proof, got %q", event.Message)
	}
	events, err := store.LoadEvents(run.ID, 100)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	for _, event := range events {
		if event.Kind == "sidecar.capsule.delivered" {
			t.Fatalf("sidecar must not be marked delivered into an already completed run: %+v", event)
		}
	}
}

func TestAttachContextMarksTerminalBoundaryWhenProviderReturnsJustBeforeCompletion(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{
		AgentID:          "opencode",
		Protocol:         "acp",
		ChannelID:        "noema.http",
		ExecutionMode:    runtrace.ExecutionModeAsync,
		LogicalSessionID: "logical-live",
		RemoteSessionID:  "remote-live",
		TracePolicy:      runtrace.TracePolicy{ContentMode: runtrace.ContentModeInline},
	})
	if err != nil {
		t.Fatalf("Start run: %v", err)
	}
	attacher := fakeAttacher{attach: func(_ context.Context, req middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		go func() {
			time.Sleep(100 * time.Millisecond)
			if _, err := store.Complete(req.RunID, "final", "end_turn"); err != nil {
				t.Errorf("Complete run: %v", err)
			}
		}()
		return middleware.RunContextAttachmentResult{Status: "delivered", Message: "queued provider response"}, nil
	}}
	_, resp := New(store, attacher, nil).Handle(context.Background(), run.ID, Request{
		Action: "attach_context",
		SidecarCapsules: []middleware.SidecarCapsule{
			{Provider: "noema", ID: "ctx", Visibility: middleware.SidecarVisibilityLLMVisible, Content: "marker"},
		},
	})
	if !resp.Accepted {
		t.Fatalf("expected accepted response, got %+v", resp)
	}
	event := waitRunActionEvent(t, store, run.ID, "run.context.attached", deliveryStatusTerminalBoundary)
	if event.Metadata["delivery_class"] != deliveryClassProviderReturnedTerminal {
		t.Fatalf("expected terminal boundary class, got %+v", event.Metadata)
	}
	if event.Metadata["live_consumption_proven"] != false {
		t.Fatalf("terminal-boundary attach must not claim live consumption: %+v", event.Metadata)
	}
	assertNoRunActionEvent(t, store, run.ID, "sidecar.capsule.delivered")
}

func TestAttachContextMarksDeliveredWhenAttachPromptStreamsActivity(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{
		AgentID:          "opencode",
		Protocol:         "acp",
		ChannelID:        "noema.http",
		ExecutionMode:    runtrace.ExecutionModeAsync,
		LogicalSessionID: "logical-live",
		RemoteSessionID:  "remote-live",
	})
	if err != nil {
		t.Fatalf("Start run: %v", err)
	}
	attacher := fakeAttacher{attach: func(_ context.Context, req middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		req.Notifier.OnThought(middleware.ThoughtUpdate{Type: middleware.ThoughtTypeThinking, Content: "agent processed live attach"})
		return middleware.RunContextAttachmentResult{Status: "delivered", Message: "agent saw marker"}, nil
	}}
	_, resp := New(store, attacher, nil).Handle(context.Background(), run.ID, Request{
		Action: "attach_context",
		SidecarCapsules: []middleware.SidecarCapsule{
			{Provider: "noema", ID: "ctx", Visibility: middleware.SidecarVisibilityLLMVisible, Content: "marker"},
		},
	})
	if !resp.Accepted {
		t.Fatalf("expected accepted response, got %+v", resp)
	}
	event := waitRunActionEvent(t, store, run.ID, "run.context.attached", deliveryStatusDelivered)
	if event.Metadata["delivery_class"] != deliveryClassLiveActivityObserved {
		t.Fatalf("expected live activity class, got %+v", event.Metadata)
	}
	if event.Metadata["live_consumption_proven"] != true {
		t.Fatalf("expected live consumption proof, got %+v", event.Metadata)
	}
	sidecarEvent := waitRunActionEvent(t, store, run.ID, "sidecar.capsule.delivered", runtrace.StatusCompleted)
	if sidecarEvent.Metadata["delivery_id"] != resp.DeliveryID {
		t.Fatalf("expected sidecar delivery id %s, got %+v", resp.DeliveryID, sidecarEvent.Metadata)
	}
}

func TestAttachContextMarksUnverifiedWhenProviderReturnsWithoutActivity(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{
		AgentID:          "opencode",
		Protocol:         "acp",
		ChannelID:        "noema.http",
		ExecutionMode:    runtrace.ExecutionModeAsync,
		LogicalSessionID: "logical-live",
		RemoteSessionID:  "remote-live",
	})
	if err != nil {
		t.Fatalf("Start run: %v", err)
	}
	attacher := fakeAttacher{attach: func(_ context.Context, _ middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		return middleware.RunContextAttachmentResult{Status: "delivered", Message: "provider returned but did not stream"}, nil
	}}
	_, resp := New(store, attacher, nil).Handle(context.Background(), run.ID, Request{
		Action: "attach_context",
		SidecarCapsules: []middleware.SidecarCapsule{
			{Provider: "noema", ID: "ctx", Visibility: middleware.SidecarVisibilityLLMVisible, Content: "marker"},
		},
	})
	if !resp.Accepted {
		t.Fatalf("expected accepted response, got %+v", resp)
	}
	event := waitRunActionEvent(t, store, run.ID, "run.context.attached", deliveryStatusUnverified)
	if event.Metadata["delivery_class"] != deliveryClassProviderReturnedUnverified {
		t.Fatalf("expected unverified class, got %+v", event.Metadata)
	}
	if event.Metadata["live_consumption_proven"] != false {
		t.Fatalf("unverified attach must not claim live consumption: %+v", event.Metadata)
	}
	assertNoRunActionEvent(t, store, run.ID, "sidecar.capsule.delivered")
}

func TestAttachContextMarksLateWhenProviderDoesNotReturn(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{
		AgentID:          "codex",
		Protocol:         "acp",
		ChannelID:        "noema.http",
		ExecutionMode:    runtrace.ExecutionModeAsync,
		LogicalSessionID: "logical-live",
		RemoteSessionID:  "remote-live",
	})
	if err != nil {
		t.Fatalf("Start run: %v", err)
	}
	attacher := fakeAttacher{attach: func(ctx context.Context, _ middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		<-ctx.Done()
		return middleware.RunContextAttachmentResult{}, ctx.Err()
	}}
	_, resp := New(store, attacher, nil).Handle(context.Background(), run.ID, Request{
		Action: "attach_context",
		SidecarCapsules: []middleware.SidecarCapsule{
			{Provider: "noema", ID: "ctx", Visibility: middleware.SidecarVisibilityLLMVisible, Content: "marker"},
		},
	})
	if !resp.Accepted {
		t.Fatalf("expected accepted response, got %+v", resp)
	}
	if _, err := store.Complete(run.ID, "final", "end_turn"); err != nil {
		t.Fatalf("Complete run: %v", err)
	}
	event := waitRunActionEvent(t, store, run.ID, "run.context.attached", "late")
	if event.Metadata["delivery_id"] != resp.DeliveryID {
		t.Fatalf("expected late event to preserve delivery id, got %+v", event.Metadata)
	}
}

func assertNoRunActionEvent(t *testing.T, store *runtrace.Store, runID, kind string) {
	t.Helper()
	events, err := store.LoadEvents(runID, 100)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	for _, event := range events {
		if event.Kind == kind {
			t.Fatalf("unexpected event %s: %+v", kind, event)
		}
	}
}

func waitRunActionEvent(t *testing.T, store *runtrace.Store, runID, kind, status string) runtrace.Event {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		events, err := store.LoadEvents(runID, 100)
		if err != nil {
			t.Fatalf("LoadEvents: %v", err)
		}
		for _, event := range events {
			if event.Kind == kind && event.Status == status {
				return event
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("event %s/%s not found", kind, status)
	return runtrace.Event{}
}
