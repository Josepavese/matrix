package runaction

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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

type fakeRunCanceller struct {
	request middleware.RunCancellationRequest
	called  bool
}

func (f *fakeRunCanceller) CancelRun(_ context.Context, req middleware.RunCancellationRequest) error {
	f.called = true
	f.request = req
	return nil
}

func TestCancelSignalsRunBoundRemoteBeforeLocalContext(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{
		AgentID: "codex", Protocol: "acp", ChannelID: "task-b",
		ExecutionMode: runtrace.ExecutionModeAsync, WorkspacePath: "/tmp/shared",
		LogicalSessionID: "logical-b", RemoteSessionID: "remote-b",
	})
	if err != nil {
		t.Fatalf("Start run: %v", err)
	}
	provider := &fakeRunCanceller{}
	localCancelled := false
	_, resp := New(store, nil, func(string) {
		if !provider.called {
			t.Fatal("local context cancelled before provider signal")
		}
		localCancelled = true
	}, provider).Handle(context.Background(), run.ID, Request{Action: "cancel"})
	if !resp.Accepted || resp.Status != runtrace.StatusCancelled || !localCancelled {
		t.Fatalf("unexpected cancel response: %+v local=%v", resp, localCancelled)
	}
	if provider.request.RunID != run.ID || provider.request.RemoteSessionID != "remote-b" || provider.request.WorkspacePath != "/tmp/shared" {
		t.Fatalf("provider cancel did not use run-bound SSOT: %+v", provider.request)
	}
	event := waitRunActionEvent(t, store, run.ID, "run.cancel.signal", runtrace.StatusCompleted)
	if event.ProtocolMethod != "session/cancel" {
		t.Fatalf("unexpected cancel signal event: %+v", event)
	}
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
	releaseProvider := make(chan struct{})
	defer func() {
		select {
		case <-releaseProvider:
		default:
			close(releaseProvider)
		}
	}()
	attacher := fakeAttacher{attach: func(_ context.Context, req middleware.RunContextAttachmentRequest) (middleware.RunContextAttachmentResult, error) {
		if _, err := store.Complete(req.RunID, "final", "end_turn"); err != nil {
			t.Fatalf("Complete run: %v", err)
		}
		<-releaseProvider
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
	first := waitRunActionEvent(t, store, run.ID, "run.context.attached", "late")
	if first.Message != "Live context delivery was still pending when the run completed." {
		t.Fatalf("watcher did not record the pending terminal delivery: %+v", first)
	}
	close(releaseProvider)
	event := waitRunActionEvent(t, store, run.ID, "run.context.attached", "late", "agent saw marker")
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
	storage := &pollObservingStorage{Storage: memstore.New(), fired: make(chan struct{})}
	store := runtrace.NewStore(storage)
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
		storage.arm()
		go func() {
			select {
			case <-storage.fired:
			case <-time.After(5 * time.Second):
				t.Errorf("the service never polled the run inside the boundary window")
				return
			}
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

func waitRunActionEvent(t *testing.T, store *runtrace.Store, runID, kind, status string, messages ...string) runtrace.Event {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		events, err := store.LoadEvents(runID, 100)
		if err != nil {
			t.Fatalf("LoadEvents: %v", err)
		}
		for _, event := range events {
			if event.Kind == kind && event.Status == status && (len(messages) == 0 || event.Message == messages[0]) {
				return event
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("event %s/%s not found", kind, status)
	return runtrace.Event{}
}

// pollObservingStorage is a test-only seam: it wraps the injected storage and
// reports the poll the service issues from inside the terminal boundary window,
// which is the read whose timing decides whether the completion this test stages
// is observed at the boundary.
//
// Reporting the first Get after arming was wrong twice over. The service reads
// the run once before the window opens - currentRunningRun decides whether the
// provider came back after completion - so the first Get was that check and not
// the poll; and the fact was signalled before the read was served, so the staged
// completion could land inside the very read being observed. The window then
// never saw the run end, the service classified the completion as having
// happened before the provider returned, and this test failed on schedules that
// let the completion goroutine run first. The read is now identified by its call
// site and reported after it has been served, so the completion lands after the
// provider-return check and before the window closes.
type pollObservingStorage struct {
	middleware.Storage
	fired chan struct{}
	once  sync.Once
	armed atomic.Bool
}

func (p *pollObservingStorage) arm() { p.armed.Store(true) }

func (p *pollObservingStorage) Get(key string) ([]byte, error) {
	data, err := p.Storage.Get(key)
	if p.armed.Load() && isBoundaryWindowPoll() {
		p.once.Do(func() { close(p.fired) })
	}
	return data, err
}

// isBoundaryWindowPoll reports whether the read being served comes from the
// terminal boundary window. It is identified by its call site on purpose: if the
// window is renamed or removed, this test fails loudly - the service never polled
// inside the window - instead of passing on a fact it did not stage.
func isBoundaryWindowPoll() bool {
	pc := make([]uintptr, 32)
	frames := runtime.CallersFrames(pc[:runtime.Callers(2, pc)])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ".runEndsBeforeBoundaryWindow") {
			return true
		}
		if !more {
			return false
		}
	}
}
