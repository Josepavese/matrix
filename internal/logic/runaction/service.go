package runaction

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/Josepavese/matrix/internal/logic/runnotifier"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/sidecar"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/google/uuid"
)

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
	s.appendAttachEvent(run, req, deliveryState{ID: deliveryID, Status: deliveryStatusAccepted, Class: deliveryClassPending})
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
		cancel:     cancel,
		recordLate: record,
	})
	notifier := newAttachProofNotifier(runnotifier.New(s.store, run.ID, run.AgentID, run.Protocol), deliveryID)
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
		Notifier:         notifier,
	})
	close(done)
	if err != nil {
		record(run, deliveryState{ID: deliveryID, Status: deliveryStatusFailed, Message: err.Error(), Class: deliveryClassProviderFailed}, false)
		return
	}
	if result.Unsupported {
		record(run, deliveryState{ID: deliveryID, Status: deliveryStatusUnsupported, Message: result.Message, Class: deliveryClassUnsupported}, false)
		return
	}
	currentRun, ok := s.currentRunningRun(run)
	if !ok {
		record(currentRun, deliveryState{ID: deliveryID, Status: deliveryStatusLate, Message: firstNonEmpty(result.Message, "Live context delivered after run completion."), Class: deliveryClassRunCompletedBeforeReturn}, false)
		return
	}
	state, emitSidecars := s.classifyProviderReturn(deliverCtx, currentRun, result, notifier)
	record(currentRun, state, emitSidecars)
}

func unsupportedResponse(runID, action, message string) Response {
	return Response{RunID: runID, Action: firstNonEmpty(action, "attach_context"), Status: "unsupported", Accepted: false, Message: message}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
