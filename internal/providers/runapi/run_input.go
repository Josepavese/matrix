package runapi

import (
	"context"
	"net/http"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/runpayload"
)

const (
	inputUnsupported           = "input_content_unsupported"
	inputCapabilityUnavailable = "input_capability_unavailable"
	inputAcceptedEvent         = "input.content.accepted"
	modelContentUnverified     = "unverified"
)

// Transport admission never claims that a selected model actually saw an image.
// The ACP adapter checks its live client again before session/prompt execution.
func (s *Server) prepareRunInput(w http.ResponseWriter, req runRequest) bool {
	if len(req.Input.Content) == 0 {
		return true
	}
	if _, ok := s.router.(middleware.ConversationRequestRouter); !ok {
		writeJSON(w, http.StatusConflict, map[string]string{"code": inputUnsupported, "message": "router does not support typed run input"})
		return false
	}
	for _, block := range req.Input.Content {
		if block.Type != runpayload.ContentImage {
			continue
		}
		if req.ModelID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": inputUnsupported, "message": "image input requires an explicit model_id"})
			return false
		}
		return true
	}
	return true
}

// Live probes belong to the executing run, after idempotent replay is resolved,
// so retries cannot start clients and admission failures share terminal cleanup.
func (s *Server) admitRunContent(ctx context.Context, req runRequest, agentID string) error {
	for _, block := range req.Input.Content {
		if block.Type == runpayload.ContentImage {
			return s.admitRunImage(ctx, agentID)
		}
	}
	return nil
}

func (s *Server) admitRunImage(ctx context.Context, agentID string) error {
	result, err := s.router.HandleSessionActionTyped(ctx, middleware.SessionActionRequest{Action: "capabilities", AgentID: agentID})
	if err != nil {
		return &inputRefusal{inputCapabilityUnavailable, http.StatusServiceUnavailable, "agent content capability probe failed"}
	}
	if result.Capabilities == nil {
		return &inputRefusal{inputCapabilityUnavailable, http.StatusConflict, "agent content capability cannot be verified"}
	}
	if !result.Capabilities.Content[runpayload.ContentImage].Supported {
		return &inputRefusal{inputUnsupported, http.StatusConflict, "agent does not advertise image input"}
	}
	return nil
}

type inputRefusal struct {
	code    string
	status  int
	message string
}

func (e *inputRefusal) Error() string       { return e.message }
func (e *inputRefusal) FailureCode() string { return e.code }

func (s *Server) appendInputEvidence(run runtrace.Run, input runpayload.Input) {
	if len(input.Content) == 0 {
		return
	}
	_, _ = s.runStore.AppendEvent(runtrace.Event{
		RunID: run.ID, Kind: inputAcceptedEvent, Actor: "matrix", Status: runtrace.StatusCompleted,
		ContentDigest: input.Digest(), ContentRef: run.InputRef,
		Metadata: map[string]interface{}{
			"parts":                      input.Manifest(),
			"model_content_verification": modelContentUnverified,
		},
	})
}
