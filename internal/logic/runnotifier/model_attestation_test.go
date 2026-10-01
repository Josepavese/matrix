package runnotifier

import (
	"context"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runactivity"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

// replayedTrace reads the run back through a fresh store over the same storage,
// which is what a consumer gets when it exports the trace after the run instead
// of watching the live notifier.
func replayedTrace(t *testing.T, backend middleware.Storage, runID string) runtrace.Trace {
	t.Helper()
	trace, found, err := runtrace.NewStore(backend).Trace(runID)
	if err != nil || !found {
		t.Fatalf("replayed trace: found=%v err=%v", found, err)
	}
	return trace
}

func modelSelectionEvent(t *testing.T, trace runtrace.Trace) runtrace.Event {
	t.Helper()
	for _, event := range trace.Events {
		if event.Kind == "model.selection" {
			return event
		}
	}
	t.Fatalf("model.selection event missing after replay: %#v", trace.Events)
	return runtrace.Event{}
}

func metadataString(t *testing.T, event runtrace.Event, key string) string {
	t.Helper()
	value, ok := event.Metadata[key]
	if !ok {
		t.Fatalf("model.selection event has no %q: %#v", key, event.Metadata)
	}
	text, _ := value.(string)
	return text
}

// TestReplayedTraceKeepsRequestedSelectedAndConfirmedApart is the trace half of
// the issue's test: after replay, the run and the model.selection event must
// state what was requested, what Matrix selected on the session, and what the
// provider confirmed, without ever promoting the requested model to a
// confirmation.
func TestReplayedTraceKeepsRequestedSelectedAndConfirmedApart(t *testing.T) {
	backend := memstore.New()
	store := runtrace.NewStore(backend)
	run, _, err := store.Start(runtrace.Run{
		AgentID: "opencode", ChannelID: "halfpocket.runs", Protocol: "acp",
		RequestedModel: "deepseek/deepseek-flash", ModelVerification: middleware.ModelVerificationUnverified,
	})
	if err != nil {
		t.Fatal(err)
	}
	New(store, run.ID, "opencode", "acp").OnModelSelection(middleware.ModelSelection{
		ConfiguredModel:    "deepseek/deepseek-flash",
		Verification:       middleware.ModelVerificationUnverified,
		VerificationReason: middleware.ModelUnverifiedNotRepeated,
		EvidenceSource:     "session/set_model",
	})

	trace := replayedTrace(t, backend, run.ID)
	if trace.Run.RequestedModel != "deepseek/deepseek-flash" {
		t.Fatalf("requested model lost after replay: %#v", trace.Run)
	}
	if trace.Run.ConfiguredModel != "deepseek/deepseek-flash" {
		t.Fatalf("selected model must be recorded even when unconfirmed: %#v", trace.Run)
	}
	if trace.Run.EffectiveModel != "" {
		t.Fatalf("requested model was promoted to a confirmation: %#v", trace.Run)
	}
	if trace.Run.ModelVerification != middleware.ModelVerificationUnverified {
		t.Fatalf("unconfirmed selection reported as %q", trace.Run.ModelVerification)
	}

	event := modelSelectionEvent(t, trace)
	for key, want := range map[string]string{
		"requested_model":     "deepseek/deepseek-flash",
		"selected_model":      "deepseek/deepseek-flash",
		"configured_model":    "deepseek/deepseek-flash",
		"confirmed_model":     "",
		"effective_model":     "",
		"verification":        middleware.ModelVerificationUnverified,
		"verification_reason": middleware.ModelUnverifiedNotRepeated,
		"evidence_source":     "session/set_model",
	} {
		if got := metadataString(t, event, key); got != want {
			t.Fatalf("event %s=%q want %q (metadata %#v)", key, got, want, event.Metadata)
		}
	}
}

// TestReplayedTracePublishesAProviderConfirmation keeps the positive case: when
// the provider stated the model, the confirmation reaches the run and the event,
// and the event carries no failure reason.
func TestReplayedTracePublishesAProviderConfirmation(t *testing.T) {
	backend := memstore.New()
	store := runtrace.NewStore(backend)
	run, _, err := store.Start(runtrace.Run{
		AgentID: "opencode", ChannelID: "halfpocket.runs", Protocol: "acp",
		RequestedModel: "chosen-model", ModelVerification: middleware.ModelVerificationUnverified,
	})
	if err != nil {
		t.Fatal(err)
	}
	New(store, run.ID, "opencode", "acp").OnModelSelection(middleware.ModelSelection{
		ConfiguredModel: "chosen-model", EffectiveModel: "chosen-model",
		Verification:   middleware.ModelVerificationConfirmed,
		EvidenceSource: "session/set_config_option",
	})

	trace := replayedTrace(t, backend, run.ID)
	if trace.Run.EffectiveModel != "chosen-model" || trace.Run.ModelVerification != middleware.ModelVerificationConfirmed {
		t.Fatalf("provider confirmation lost after replay: %#v", trace.Run)
	}
	event := modelSelectionEvent(t, trace)
	if got := metadataString(t, event, "confirmed_model"); got != "chosen-model" {
		t.Fatalf("confirmed_model=%q in %#v", got, event.Metadata)
	}
	if got := metadataString(t, event, "verification_reason"); got != "" {
		t.Fatalf("a confirmed selection must carry no failure reason, got %q", got)
	}
}

// TestRunPathCompositionReachesTheTraceWithModelEvidence is the composition the
// run API builds: a run notifier decorated by the activity watchdog. The
// decorator must not hide the optional capability that carries model evidence,
// or every run with an activity timeout silently reports an unverified model
// with no selected model at all.
func TestRunPathCompositionReachesTheTraceWithModelEvidence(t *testing.T) {
	backend := memstore.New()
	store := runtrace.NewStore(backend)
	run, _, err := store.Start(runtrace.Run{
		AgentID: "opencode", ChannelID: "halfpocket.runs", Protocol: "acp",
		RequestedModel: "deepseek/deepseek-flash", ModelVerification: middleware.ModelVerificationUnverified,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := New(store, run.ID, "opencode", "acp")
	_, wrapped, _, stop := runactivity.WithTimeout(context.Background(), time.Minute, base)
	defer stop()

	forwarder, ok := wrapped.(middleware.ModelSelectionNotifier)
	if !ok {
		t.Fatal("the run path lost the model selection notifier before the adapter could publish evidence")
	}
	forwarder.OnModelSelection(middleware.ModelSelection{
		ConfiguredModel: "deepseek/deepseek-flash", EvidenceSource: "session/set_model",
		Verification: middleware.ModelVerificationUnverified, VerificationReason: middleware.ModelUnverifiedProviderDoesNotAttest,
	})

	trace := replayedTrace(t, backend, run.ID)
	if trace.Run.ConfiguredModel != "deepseek/deepseek-flash" {
		t.Fatalf("selected model never reached the trace: %#v", trace.Run)
	}
	if trace.Run.EffectiveModel != "" || trace.Run.ModelVerification != middleware.ModelVerificationUnverified {
		t.Fatalf("unverified evidence was reported as a confirmation: %#v", trace.Run)
	}
	if got := metadataString(t, modelSelectionEvent(t, trace), "verification_reason"); got != middleware.ModelUnverifiedProviderDoesNotAttest {
		t.Fatalf("verification_reason=%q", got)
	}
}
