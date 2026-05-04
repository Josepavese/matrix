package runaction

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/sidecartrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

const (
	deliveryStatusAccepted         = "accepted"
	deliveryStatusDelivered        = "delivered"
	deliveryStatusFailed           = "failed"
	deliveryStatusLate             = "late"
	deliveryStatusTerminalBoundary = "terminal_boundary"
	deliveryStatusUnverified       = "unverified"
	deliveryStatusUnsupported      = "unsupported"

	deliveryClassPending                    = "pending"
	deliveryClassLiveActivityObserved       = "live_activity_observed"
	deliveryClassProviderReturnedUnverified = "provider_returned_unverified"
	deliveryClassProviderReturnedTerminal   = "provider_returned_terminal_boundary"
	deliveryClassRunCompletedBeforeReturn   = "run_completed_before_provider_return"
	deliveryClassProviderFailed             = "provider_failed"
	deliveryClassUnsupported                = "unsupported"
)

const (
	attachTerminalPollInterval    = 500 * time.Millisecond
	attachTerminalBoundaryWindow  = 2 * time.Second
	attachDeliveryObserveInterval = 50 * time.Millisecond
)

type deliveryState struct {
	ID                    string
	Status                string
	Message               string
	Class                 string
	LiveConsumptionProven bool
	ActivityCount         int
	ObserveWindow         time.Duration
}

type deliveryRecorder func(runtrace.Run, deliveryState, bool)

type deliveryWatch struct {
	run        runtrace.Run
	deliveryID string
	done       <-chan struct{}
	cancel     context.CancelFunc
	recordLate deliveryRecorder
}

type attachProofNotifier struct {
	base       middleware.ThoughtNotifier
	deliveryID string
	mu         sync.Mutex
	count      int
}

func newAttachProofNotifier(base middleware.ThoughtNotifier, deliveryID string) *attachProofNotifier {
	return &attachProofNotifier{base: base, deliveryID: deliveryID}
}

func (n *attachProofNotifier) OnThought(update middleware.ThoughtUpdate) {
	n.mu.Lock()
	n.count++
	n.mu.Unlock()
	update.Metadata = cloneMetadata(update.Metadata)
	update.Metadata["matrix_delivery_id"] = n.deliveryID
	update.Metadata["matrix_delivery_source"] = "live_attach"
	if n.base != nil {
		n.base.OnThought(update)
	}
}

func (n *attachProofNotifier) SetHeader(agentID, agentSessionID string) {
	if n.base != nil {
		n.base.SetHeader(agentID, agentSessionID)
	}
}

func (n *attachProofNotifier) FormattedHeader() string {
	if n.base == nil {
		return ""
	}
	return n.base.FormattedHeader()
}

func (n *attachProofNotifier) ActivityCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.count
}

func cloneMetadata(meta map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range meta {
		out[k] = v
	}
	return out
}

func (s Service) classifyProviderReturn(ctx context.Context, run runtrace.Run, result middleware.RunContextAttachmentResult, notifier *attachProofNotifier) (deliveryState, bool) {
	state := deliveryState{
		ID:                    firstNonEmpty(result.DeliveryID, notifier.deliveryID),
		Status:                deliveryStatusDelivered,
		Message:               result.Message,
		Class:                 firstNonEmpty(result.DeliveryClass, deliveryClassProviderReturnedUnverified),
		LiveConsumptionProven: result.LiveConsumptionProven,
		ActivityCount:         result.ProviderActivityEvents + notifier.ActivityCount(),
		ObserveWindow:         attachTerminalBoundaryWindow,
	}
	if state.ActivityCount > 0 {
		state.Class = deliveryClassLiveActivityObserved
		state.LiveConsumptionProven = true
		return state, true
	}
	if state.LiveConsumptionProven {
		state.Class = firstNonEmpty(result.DeliveryClass, deliveryClassLiveActivityObserved)
		return state, true
	}
	if s.runEndsBeforeBoundaryWindow(ctx, run.ID) {
		state.Status = deliveryStatusTerminalBoundary
		state.Class = deliveryClassProviderReturnedTerminal
		state.Message = firstNonEmpty(state.Message, "Live context provider returned at the terminal boundary without live activity proof.")
		return state, false
	}
	state.Status = deliveryStatusUnverified
	state.Message = firstNonEmpty(state.Message, "Live context provider returned without live activity proof.")
	return state, false
}

func (s Service) runEndsBeforeBoundaryWindow(ctx context.Context, runID string) bool {
	timer := time.NewTimer(attachTerminalBoundaryWindow)
	defer timer.Stop()
	ticker := time.NewTicker(attachDeliveryObserveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return true
		case <-timer.C:
			return false
		case <-ticker.C:
			run, found, err := s.store.LoadRun(runID)
			if err != nil || !found {
				return true
			}
			if run.Status != runtrace.StatusRunning {
				return true
			}
		}
	}
}

func (s Service) recordAttach(run runtrace.Run, req Request, state deliveryState, emitSidecars bool) {
	if emitSidecars {
		s.appendDeliveredSidecars(run, req, state)
	}
	s.appendAttachEvent(run, req, state)
}

func (s Service) appendDeliveredSidecars(run runtrace.Run, req Request, state deliveryState) {
	for _, event := range sidecartrace.Events(run, req.SidecarCapsules) {
		event.Metadata = mergeMeta(event.Metadata, attachMetadata(req, state))
		_, _ = s.store.AppendEvent(event)
	}
}

func (s Service) watchRunTerminal(ctx context.Context, watch deliveryWatch) {
	ticker := time.NewTicker(attachTerminalPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-watch.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			current, ok := s.currentRunningRun(watch.run)
			if ok {
				continue
			}
			watch.cancel()
			watch.recordLate(current, deliveryState{ID: watch.deliveryID, Status: deliveryStatusLate, Message: "Live context delivery was still pending when the run completed.", Class: deliveryClassRunCompletedBeforeReturn}, false)
			return
		}
	}
}

func (s Service) currentRunningRun(run runtrace.Run) (runtrace.Run, bool) {
	current, found, err := s.store.LoadRun(run.ID)
	if err != nil || !found {
		return run, false
	}
	if current.Status != runtrace.StatusRunning {
		return current, false
	}
	return current, true
}

func (s Service) appendAttachEvent(run runtrace.Run, req Request, state deliveryState) {
	event := runtrace.Event{
		RunID:          run.ID,
		Kind:           "run.context.attached",
		Actor:          "matrix",
		Status:         state.Status,
		Protocol:       run.Protocol,
		ProtocolMethod: "matrix.run.context.attach",
		Summary:        attachSummary(state),
		Metadata:       attachRunMetadata(run, attachMetadata(req, state)),
	}
	if run.TracePolicy.ContentMode == runtrace.ContentModeInline {
		event.Message = state.Message
	}
	_, _ = s.store.AppendEvent(event)
}

func attachSummary(state deliveryState) string {
	if state.Status == deliveryStatusDelivered {
		return "Live context delivered to active session."
	}
	if state.Status == deliveryStatusTerminalBoundary {
		return "Live context reached provider at terminal boundary without live proof."
	}
	if state.Status == deliveryStatusUnverified {
		return "Live context provider returned without live proof."
	}
	return state.Message
}

func attachMetadata(req Request, state deliveryState) map[string]interface{} {
	meta := map[string]interface{}{
		"delivery_id":              state.ID,
		"delivery_status":          state.Status,
		"delivery_class":           firstNonEmpty(state.Class, state.Status),
		"live_consumption_proven":  state.LiveConsumptionProven,
		"provider_activity_events": state.ActivityCount,
		"reason":                   strings.TrimSpace(req.Reason),
		"capsule_count":            len(req.SidecarCapsules),
		"frontend_visible":         false,
		"audit_visible":            true,
		"trace_visible":            true,
	}
	if req.SourceEventID != "" {
		meta["source_event_id"] = strings.TrimSpace(req.SourceEventID)
	}
	if req.SourceSequence > 0 {
		meta["source_sequence"] = req.SourceSequence
	}
	if state.Message != "" {
		meta["message"] = state.Message
	}
	if state.ObserveWindow > 0 {
		meta["terminal_boundary_window_ms"] = int(state.ObserveWindow / time.Millisecond)
	}
	return meta
}

func attachRunMetadata(run runtrace.Run, meta map[string]interface{}) map[string]interface{} {
	if strings.TrimSpace(run.LogicalSessionID) != "" {
		meta["logical_session_id"] = strings.TrimSpace(run.LogicalSessionID)
	}
	if strings.TrimSpace(run.RemoteSessionID) != "" {
		meta["remote_session_id"] = strings.TrimSpace(run.RemoteSessionID)
	}
	if strings.TrimSpace(run.AgentID) != "" {
		meta["agent_id"] = strings.TrimSpace(run.AgentID)
	}
	if strings.TrimSpace(run.WorkspaceID) != "" {
		meta["workspace_id"] = strings.TrimSpace(run.WorkspaceID)
	}
	return meta
}

func mergeMeta(a, b map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
