package runtrace

import (
	"fmt"
	"sync"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
)

func TestStoreProjectsTrace(t *testing.T) {
	store := NewStore(memstore.New())
	run, _, err := store.Start(Run{
		AgentID:       "codex",
		Protocol:      "acp",
		WorkspaceID:   "repo-main",
		ChannelID:     "http.test",
		ExecutionMode: ExecutionModeSync,
		InputRef:      "matrix://runs/pending/input",
		TracePolicy:   TracePolicy{ContentMode: ContentModeRefs, RedactionProfile: "default", IncludeProtocolMeta: true},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := store.AppendEvent(Event{RunID: run.ID, Kind: "routing.decision", Actor: "matrix", Status: StatusCompleted}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if _, err := store.Complete(run.ID, "done", "end_turn"); err != nil {
		t.Fatalf("complete run: %v", err)
	}

	trace, found, err := store.Trace(run.ID)
	if err != nil {
		t.Fatalf("trace: %v", err)
	}
	if !found {
		t.Fatal("expected trace to be found")
	}
	if trace.Schema != SchemaAgentCommunicationRunTraceV0 {
		t.Fatalf("unexpected schema: %s", trace.Schema)
	}
	if trace.Run.Status != StatusCompleted {
		t.Fatalf("unexpected status: %s", trace.Run.Status)
	}
	if trace.Surface.Redaction != "content_ref_only" {
		t.Fatalf("unexpected redaction: %s", trace.Surface.Redaction)
	}
	if len(trace.Events) < 4 {
		t.Fatalf("expected recorded run events, got %d", len(trace.Events))
	}
}

func TestStoreDefaultsEmptyTracePolicyToProtocolMetaIncluded(t *testing.T) {
	store := NewStore(memstore.New())
	run, _, err := store.Start(Run{AgentID: "opencode", ChannelID: "http.test", ExecutionMode: ExecutionModeSync})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if run.TracePolicy.ContentMode != ContentModeRefs {
		t.Fatalf("unexpected content mode: %s", run.TracePolicy.ContentMode)
	}
	if run.TracePolicy.RedactionProfile != "default" {
		t.Fatalf("unexpected redaction profile: %s", run.TracePolicy.RedactionProfile)
	}
	if !run.TracePolicy.IncludeProtocolMeta {
		t.Fatal("expected empty trace policy to include protocol metadata")
	}
}

func TestStorePreservesExplicitProtocolMetaExclusion(t *testing.T) {
	store := NewStore(memstore.New())
	run, _, err := store.Start(Run{
		AgentID:       "opencode",
		ChannelID:     "http.test",
		ExecutionMode: ExecutionModeSync,
		TracePolicy:   TracePolicy{ContentMode: ContentModeRedacted, RedactionProfile: "strict", IncludeProtocolMeta: false},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if run.TracePolicy.IncludeProtocolMeta {
		t.Fatal("expected explicit protocol metadata exclusion to be preserved")
	}
}

func TestInlineTraceProjectionContainsFrontendFinalContent(t *testing.T) {
	store := NewStore(memstore.New())
	run, _, err := store.Start(Run{
		AgentID:     "opencode",
		Protocol:    "acp",
		ChannelID:   "noema-zed",
		TracePolicy: TracePolicy{ContentMode: ContentModeInline, RedactionProfile: "frontend", IncludeProtocolMeta: false},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := store.Complete(run.ID, "hello from opencode", "end_turn"); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	trace, found, err := store.Trace(run.ID)
	if err != nil || !found {
		t.Fatalf("trace found=%v err=%v", found, err)
	}
	if trace.Outcome.Summary != "hello from opencode" {
		t.Fatalf("expected inline summary, got %q", trace.Outcome.Summary)
	}
	foundFinal := false
	for _, event := range trace.Events {
		if event.Kind != "agent.message.final" {
			continue
		}
		foundFinal = true
		if event.Message != "hello from opencode" {
			t.Fatalf("expected final event inline message, got %q", event.Message)
		}
		if event.ProtocolMeta != nil {
			t.Fatalf("expected protocol metadata to be excluded, got %#v", event.ProtocolMeta)
		}
	}
	if !foundFinal {
		t.Fatal("expected agent.message.final event")
	}
}

func TestRedactedTracePreservesSidecarVisibilityMetadata(t *testing.T) {
	store := NewStore(memstore.New())
	run, _, err := store.Start(Run{
		AgentID:     "opencode",
		Protocol:    "acp",
		ChannelID:   "noema.http",
		TracePolicy: TracePolicy{ContentMode: ContentModeRedacted, RedactionProfile: "frontend", IncludeProtocolMeta: false},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := store.AppendEvent(Event{
		RunID:             run.ID,
		Kind:              "sidecar.capsule.delivered",
		Actor:             "matrix",
		Status:            StatusCompleted,
		SidecarProvider:   "noema",
		SidecarID:         "caps-redacted",
		SidecarSchema:     "sidecar.intent.v0",
		SidecarVisibility: "llm_visible",
		Message:           "<noema>secret</noema>",
		Metadata: map[string]interface{}{
			"frontend_visible": false,
			"audit_visible":    true,
			"trace_visible":    true,
			"intent":           "secret",
		},
	}); err != nil {
		t.Fatalf("append sidecar event: %v", err)
	}

	trace, found, err := store.Trace(run.ID)
	if err != nil || !found {
		t.Fatalf("trace found=%v err=%v", found, err)
	}
	var sidecar *Event
	for i := range trace.Events {
		if trace.Events[i].Kind == "sidecar.capsule.delivered" {
			sidecar = &trace.Events[i]
			break
		}
	}
	if sidecar == nil {
		t.Fatal("expected sidecar event")
	}
	if sidecar.Message != "" || sidecar.Summary != "" {
		t.Fatalf("expected redacted sidecar content, got message=%q summary=%q", sidecar.Message, sidecar.Summary)
	}
	if sidecar.SidecarProvider != "noema" || sidecar.SidecarID != "caps-redacted" {
		t.Fatalf("expected sidecar identity to survive redaction, got %+v", sidecar)
	}
	if sidecar.Metadata["frontend_visible"] != false || sidecar.Metadata["audit_visible"] != true || sidecar.Metadata["trace_visible"] != true {
		t.Fatalf("expected visibility metadata to survive redaction, got %+v", sidecar.Metadata)
	}
	if _, ok := sidecar.Metadata["intent"]; ok {
		t.Fatalf("expected provider metadata to be redacted, got %+v", sidecar.Metadata)
	}
}

func TestStoreRegisterSinkRequiresHTTPURL(t *testing.T) {
	store := NewStore(memstore.New())
	if _, err := store.RegisterSink(Sink{URL: "file:///tmp/sink"}); err == nil {
		t.Fatal("expected non-http sink url to fail")
	}
	if _, err := store.RegisterSink(Sink{URL: "http://127.0.0.1:8080/events"}); err != nil {
		t.Fatalf("expected http sink url to pass: %v", err)
	}
}

func TestStoreCancelRun(t *testing.T) {
	store := NewStore(memstore.New())
	run, _, err := store.Start(Run{AgentID: "gemini", ChannelID: "telegram.user", ExecutionMode: ExecutionModeAsync})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	cancelled, err := store.Cancel(run.ID, "consumer_policy")
	if err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if cancelled.Status != StatusCancelled {
		t.Fatalf("unexpected status: %s", cancelled.Status)
	}
	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	if events[len(events)-1].Kind != "run.cancelled" {
		t.Fatalf("expected cancellation event, got %s", events[len(events)-1].Kind)
	}
}

func TestStoreAppendEventConcurrentPreservesIndex(t *testing.T) {
	store := NewStore(memstore.New())
	run, _, err := store.Start(Run{AgentID: "opencode", ChannelID: "http.test", ExecutionMode: ExecutionModeSync})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}

	const eventCount = 100
	var wg sync.WaitGroup
	errs := make(chan error, eventCount)
	for i := 0; i < eventCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := store.AppendEvent(Event{
				RunID:  run.ID,
				Kind:   fmt.Sprintf("agent.message.delta.%03d", i),
				Actor:  "opencode",
				Status: "streaming",
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("append event: %v", err)
		}
	}

	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	if got, want := len(events), eventCount+1; got != want {
		t.Fatalf("expected %d indexed events, got %d", want, got)
	}
	for i, event := range events {
		if event.Sequence != i+1 {
			t.Fatalf("expected sequence %d at index %d, got %d", i+1, i, event.Sequence)
		}
	}
}
