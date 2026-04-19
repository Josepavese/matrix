package runaction

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jose/matrix-v2/internal/logic/runnotifier"
	"github.com/jose/matrix-v2/internal/logic/runtrace"
	"github.com/jose/matrix-v2/internal/logic/sidecar"
	"github.com/jose/matrix-v2/internal/logic/sidecartrace"
	"github.com/jose/matrix-v2/internal/middleware"
)

const attachTerminalPollInterval = 500 * time.Millisecond

type Request struct {
	Action          string                      `json:"action"`
	Reason          string                      `json:"reason,omitempty"`
	SidecarCapsules []middleware.SidecarCapsule `json:"sidecar_capsules,omitempty"`
	SourceEventID   string                      `json:"source_event_id,omitempty"`
	SourceSequence  int                         `json:"source_sequence,omitempty"`
}

type Response struct {
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	Action     string `json:"action"`
	Accepted   bool   `json:"accepted"`
	DeliveryID string `json:"delivery_id,omitempty"`
	Message    string `json:"message,omitempty"`
}

type Service struct {
	store    *runtrace.Store
	attacher middleware.RunContextAttacher
	cancel   func(string)
}

type deliveryState struct {
	ID      string
	Status  string
	Message string
}

type deliveryRecorder func(runtrace.Run, deliveryState, bool)

type deliveryWatch struct {
	run        runtrace.Run
	deliveryID string
	done       <-chan struct{}
	cancel     context.CancelFunc
	recordLate deliveryRecorder
}

func New(store *runtrace.Store, attacher middleware.RunContextAttacher, cancel func(string)) Service {
	return Service{store: store, attacher: attacher, cancel: cancel}
}

func (s Service) Handle(ctx context.Context, runID string, req Request) (int, Response) {
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "cancel", "stop":
		return s.cancelRun(runID, req)
	case "attach_context", "append_context":
		return s.attachContext(ctx, runID, req)
	default:
		return http.StatusBadRequest, Response{RunID: runID, Action: req.Action, Status: "unsupported", Message: "unsupported action"}
	}
}

func (s Service) cancelRun(runID string, req Request) (int, Response) {
	if s.cancel != nil {
		s.cancel(runID)
	}
	run, err := s.store.Cancel(runID, req.Reason)
	if err != nil {
		return http.StatusInternalServerError, Response{RunID: runID, Action: "cancel", Status: "failed", Message: err.Error()}
	}
	return http.StatusAccepted, Response{RunID: run.ID, Status: run.Status, Action: "cancel", Accepted: true}
}

func (s Service) attachContext(ctx context.Context, runID string, req Request) (int, Response) {
	req.Action = "attach_context"
	req.SidecarCapsules = sidecar.NormalizeCapsules(req.SidecarCapsules)
	run, code, resp, ok := s.validateAttach(runID, req)
	if !ok {
		if resp.Status == "unsupported" && run.ID != "" {
			s.appendAttachEvent(run, req, deliveryState{Status: "unsupported", Message: resp.Message})
		}
		return code, resp
	}
	if s.attacher == nil {
		return http.StatusAccepted, unsupportedResponse(runID, req.Action, "runtime does not support live context attachment")
	}
	deliveryID := "ctx-" + uuid.NewString()
	s.appendAttachEvent(run, req, deliveryState{ID: deliveryID, Status: "accepted"})
	go s.deliver(ctx, run, req, deliveryID)
	return http.StatusAccepted, Response{RunID: run.ID, Status: run.Status, Action: "attach_context", Accepted: true, DeliveryID: deliveryID}
}

func (s Service) validateAttach(runID string, req Request) (runtrace.Run, int, Response, bool) {
	run, found, err := s.store.LoadRun(runID)
	if err != nil {
		return runtrace.Run{}, http.StatusInternalServerError, Response{RunID: runID, Action: req.Action, Status: "failed", Message: err.Error()}, false
	}
	if !found {
		return runtrace.Run{}, http.StatusNotFound, Response{RunID: runID, Action: req.Action, Status: "not_found", Message: "run not found"}, false
	}
	if run.Status != runtrace.StatusRunning {
		return run, http.StatusConflict, Response{RunID: run.ID, Action: "attach_context", Status: "unsupported", Message: "run is not active"}, false
	}
	if len(req.SidecarCapsules) == 0 {
		return run, http.StatusBadRequest, Response{RunID: run.ID, Action: "attach_context", Status: "failed", Message: "sidecar_capsules are required"}, false
	}
	if err := sidecar.ValidateCapsules(req.SidecarCapsules); err != nil {
		return run, http.StatusBadRequest, Response{RunID: run.ID, Action: "attach_context", Status: "failed", Message: err.Error()}, false
	}
	if strings.TrimSpace(run.LogicalSessionID) == "" || strings.TrimSpace(run.RemoteSessionID) == "" {
		return run, http.StatusAccepted, unsupportedResponse(run.ID, "attach_context", "run session is not ready"), false
	}
	return run, 0, Response{}, true
}

func (s Service) deliver(ctx context.Context, run runtrace.Run, req Request, deliveryID string) {
	deliverCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	done := make(chan struct{})
	var once sync.Once
	record := func(target runtrace.Run, state deliveryState, emitSidecars bool) {
		once.Do(func() { s.recordAttach(target, req, state, emitSidecars) })
	}
	go s.watchRunTerminal(deliverCtx, deliveryWatch{
		run:        run,
		deliveryID: deliveryID,
		done:       done,
		cancel: func() {
			cancel()
		},
		recordLate: record,
	})
	result, err := s.attacher.AttachRunContext(deliverCtx, middleware.RunContextAttachmentRequest{
		RunID:            run.ID,
		DeliveryID:       deliveryID,
		ChannelID:        run.ChannelID,
		AgentID:          run.AgentID,
		WorkspaceID:      run.WorkspaceID,
		WorkspacePath:    run.WorkspacePath,
		LogicalSessionID: run.LogicalSessionID,
		RemoteSessionID:  run.RemoteSessionID,
		Reason:           req.Reason,
		SidecarCapsules:  req.SidecarCapsules,
		Notifier:         runnotifier.New(s.store, run.ID, run.AgentID, run.Protocol),
	})
	close(done)
	if err != nil {
		record(run, deliveryState{ID: deliveryID, Status: "failed", Message: err.Error()}, false)
		return
	}
	if result.Unsupported {
		record(run, deliveryState{ID: deliveryID, Status: "unsupported", Message: result.Message}, false)
		return
	}
	currentRun, ok := s.currentRunningRun(run)
	if !ok {
		record(currentRun, deliveryState{ID: deliveryID, Status: "late", Message: firstNonEmpty(result.Message, "Live context delivered after run completion.")}, false)
		return
	}
	record(currentRun, deliveryState{ID: deliveryID, Status: "delivered", Message: result.Message}, true)
}

func (s Service) recordAttach(run runtrace.Run, req Request, state deliveryState, emitSidecars bool) {
	if emitSidecars {
		s.appendDeliveredSidecars(run, req, state.ID)
	}
	s.appendAttachEvent(run, req, state)
}

func (s Service) appendDeliveredSidecars(run runtrace.Run, req Request, deliveryID string) {
	for _, event := range sidecartrace.Events(run, req.SidecarCapsules) {
		event.Metadata = mergeMeta(event.Metadata, attachMetadata(req, deliveryID, "delivered", ""))
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
			watch.recordLate(current, deliveryState{ID: watch.deliveryID, Status: "late", Message: "Live context delivery was still pending when the run completed."}, false)
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
		Metadata:       attachMetadata(req, state.ID, state.Status, state.Message),
	}
	if run.TracePolicy.ContentMode == runtrace.ContentModeInline {
		event.Message = state.Message
	}
	_, _ = s.store.AppendEvent(event)
}

func attachSummary(state deliveryState) string {
	if state.Status == "delivered" {
		return "Live context delivered to active session."
	}
	return state.Message
}

func attachMetadata(req Request, deliveryID, status, message string) map[string]interface{} {
	meta := map[string]interface{}{
		"delivery_id":      deliveryID,
		"delivery_status":  status,
		"reason":           strings.TrimSpace(req.Reason),
		"capsule_count":    len(req.SidecarCapsules),
		"frontend_visible": false,
		"audit_visible":    true,
		"trace_visible":    true,
	}
	if req.SourceEventID != "" {
		meta["source_event_id"] = strings.TrimSpace(req.SourceEventID)
	}
	if req.SourceSequence > 0 {
		meta["source_sequence"] = req.SourceSequence
	}
	if message != "" {
		meta["message"] = message
	}
	return meta
}

func unsupportedResponse(runID, action, message string) Response {
	return Response{RunID: runID, Action: firstNonEmpty(action, "attach_context"), Status: "unsupported", Accepted: false, Message: message}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
