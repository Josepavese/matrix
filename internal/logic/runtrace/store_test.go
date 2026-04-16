package runtrace

import (
	"fmt"
	"sync"
	"testing"
)

func TestStoreProjectsTrace(t *testing.T) {
	store := NewStore(NewMemoryStorage())
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
	store := NewStore(NewMemoryStorage())
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
	store := NewStore(NewMemoryStorage())
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

func TestStoreRegisterSinkRequiresHTTPURL(t *testing.T) {
	store := NewStore(NewMemoryStorage())
	if _, err := store.RegisterSink(Sink{URL: "file:///tmp/sink"}); err == nil {
		t.Fatal("expected non-http sink url to fail")
	}
	if _, err := store.RegisterSink(Sink{URL: "http://127.0.0.1:8080/events"}); err != nil {
		t.Fatalf("expected http sink url to pass: %v", err)
	}
}

func TestStoreCancelRun(t *testing.T) {
	store := NewStore(NewMemoryStorage())
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
	store := NewStore(NewMemoryStorage())
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
}
