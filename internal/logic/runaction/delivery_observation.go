package runaction

import (
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentdoctor"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

// attachEventKind is the record the attach path writes for one delivery attempt.
// The writer still spells it out; a test here drives a real attach and compares
// what it wrote against this constant, so the two cannot drift apart silently.
const attachEventKind = "run.context.attached"

// LatestAttachObservation reads what Matrix recorded the last time it attached
// live context for one agent: the status, the class and the proof flag the
// attach path wrote, in the order the attach path wrote them.
//
// The provider and delivery levels are read, never deduced. A record answers for
// the provider only once the provider returned: an attach that is still pending
// has not been answered yet, and one that failed was never answered at all, so
// both leave the capability unknown. The recorded class travels with the
// observation so a caller can say which record it read instead of guessing why
// the levels are unknown.
func LatestAttachObservation(storage middleware.Storage, agentID string) (agentdoctor.AttachObservation, error) {
	run, found, err := latestAttachedRun(storage, strings.TrimSpace(agentID))
	if err != nil || !found {
		return agentdoctor.AttachObservation{}, err
	}
	event, found, err := latestAttachEvent(storage, run.ID)
	if err != nil || !found {
		return agentdoctor.AttachObservation{}, err
	}
	return observationOfAttachEvent(event), nil
}

// latestAttachedRun is the most recently started run Matrix recorded for the
// agent. Runs are listed through RunKey(""), the prefix every stored run key
// begins with; a test pins that property, because a silent change there would
// leave every agent reporting an unknown capability instead of failing loudly.
func latestAttachedRun(storage middleware.Storage, agentID string) (runtrace.Run, bool, error) {
	if storage == nil || agentID == "" {
		return runtrace.Run{}, false, nil
	}
	prefix := runtrace.RunKey("")
	keys, err := storage.List(prefix)
	if err != nil {
		return runtrace.Run{}, false, err
	}
	store := runtrace.NewStore(storage)
	var latest runtrace.Run
	found := false
	for _, key := range keys {
		runID := strings.TrimPrefix(key, prefix)
		if runID == "" {
			continue
		}
		run, ok, err := store.LoadRun(runID)
		if err != nil {
			return runtrace.Run{}, false, err
		}
		if !ok || strings.TrimSpace(run.AgentID) != agentID {
			continue
		}
		if !found || run.StartedAt.After(latest.StartedAt) {
			latest, found = run, true
		}
	}
	return latest, found, nil
}

// latestAttachEvent is the last delivery the run recorded, or nothing when the
// run never attempted an attach.
func latestAttachEvent(storage middleware.Storage, runID string) (runtrace.Event, bool, error) {
	events, err := runtrace.NewStore(storage).LoadEvents(runID, 0)
	if err != nil {
		return runtrace.Event{}, false, err
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == attachEventKind {
			return events[i], true, nil
		}
	}
	return runtrace.Event{}, false, nil
}

// observationOfAttachEvent turns one recorded delivery into the observation the
// doctor reasons about.
func observationOfAttachEvent(event runtrace.Event) agentdoctor.AttachObservation {
	status := metadataText(event.Metadata, "delivery_status")
	class := metadataText(event.Metadata, "delivery_class")
	if !providerAnswered(status) {
		return agentdoctor.AttachObservation{DeliveryClass: firstNonEmpty(class, status)}
	}
	return agentdoctor.AttachObservation{
		Attempted:             true,
		Unsupported:           status == deliveryStatusUnsupported,
		LiveConsumptionProven: metadataFlag(event.Metadata, "live_consumption_proven"),
		ActivityCount:         metadataCount(event.Metadata, "provider_activity_events"),
		DeliveryClass:         class,
	}
}

// providerAnswered reports whether a recorded status holds an answer from the
// provider about live context. "accepted" is Matrix taking the request, not the
// provider answering it, and "failed" is no answer at all, so neither is
// evidence about the capability.
func providerAnswered(status string) bool {
	switch status {
	case deliveryStatusDelivered, deliveryStatusLate, deliveryStatusTerminalBoundary,
		deliveryStatusUnverified, deliveryStatusUnsupported:
		return true
	}
	return false
}

func metadataText(metadata map[string]interface{}, key string) string {
	text, _ := metadata[key].(string)
	return strings.TrimSpace(text)
}

func metadataFlag(metadata map[string]interface{}, key string) bool {
	flag, _ := metadata[key].(bool)
	return flag
}

func metadataCount(metadata map[string]interface{}, key string) int {
	switch count := metadata[key].(type) {
	case int:
		return count
	case float64:
		return int(count)
	}
	return 0
}
