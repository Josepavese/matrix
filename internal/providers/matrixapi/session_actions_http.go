package matrixapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/Josepavese/matrix/internal/logic/runconfig"
	"github.com/Josepavese/matrix/internal/middleware"
)

// HandleSessionActions is the typed HTTP handler for session lifecycle actions.
func (s *Server) HandleSessionActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requireJSONContentType(w, r) {
		return
	}
	if !requireAPIKey(w, r, s.apiKey) {
		return
	}

	req, ok := decodeSessionActionRequest(w, r)
	if !ok {
		return
	}
	result, err := s.router.HandleSessionActionTyped(r.Context(), req)
	if err != nil {
		writeSessionActionFailure(w, req, err)
		return
	}
	writeSessionActionResult(w, result)
}

func decodeSessionActionRequest(w http.ResponseWriter, r *http.Request) (middleware.SessionActionRequest, bool) {
	var req sessionActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
		return middleware.SessionActionRequest{}, false
	}
	if req.ChannelID == "" || req.Action == "" {
		http.Error(w, "Bad Request: channel_id and action are required", http.StatusBadRequest)
		return middleware.SessionActionRequest{}, false
	}
	additionalDirectories, err := runconfig.NormalizeAdditionalDirectories(req.AdditionalDirectories)
	if err != nil {
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return middleware.SessionActionRequest{}, false
	}
	return middleware.SessionActionRequest{
		ChannelID:             req.ChannelID,
		Action:                req.Action,
		AgentID:               req.AgentID,
		Target:                req.Target,
		WorkspaceID:           req.WorkspaceID,
		WorkspacePath:         req.WorkspacePath,
		AdditionalDirectories: additionalDirectories,
		Ephemeral:             req.Ephemeral,
		CleanupPolicy:         req.CleanupPolicy,
		ForceForgetLocal:      req.ForceForgetLocal,
		MakeActive:            req.MakeActive,
		RestoreParent:         req.RestoreParent,
		Async:                 req.Async,
		Input:                 req.Input,
	}, true
}

func writeSessionActionFailure(w http.ResponseWriter, req middleware.SessionActionRequest, err error) {
	slog.Error("matrix session action failed", "error", err, "action", req.Action)
	var coded interface{ SessionActionCode() string }
	if req.Action != "import" || !errors.As(err, &coded) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	writeSessionActionResult(w, middleware.SessionActionResult{Action: "import", Error: &middleware.SessionActionError{Code: coded.SessionActionCode(), Message: err.Error(), Target: req.Target}})
}

func writeSessionActionResult(w http.ResponseWriter, result middleware.SessionActionResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sessionActionHTTPStatus(result))
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("matrix session action failed to encode response", "error", err)
	}
}

func sessionActionHTTPStatus(result middleware.SessionActionResult) int {
	if result.Error == nil {
		return http.StatusCreated
	}
	if status, ok := sessionActionErrorStatus[result.Error.Code]; ok {
		return status
	}
	return http.StatusBadRequest
}

var sessionActionErrorStatus = map[string]int{
	"invalid_request":        http.StatusBadRequest,
	"not_found":              http.StatusNotFound,
	"workspace_mismatch":     http.StatusConflict,
	"resume_unsupported":     http.StatusFailedDependency,
	"provider_auth_required": http.StatusFailedDependency,
	"provider_failure":       http.StatusBadGateway,
	"agent_not_found":        http.StatusNotFound,
	"cleanup_clean_without_remote_or_process_proof": http.StatusConflict,
	"cleanup_failed":                    http.StatusBadGateway,
	"cleanup_warning":                   http.StatusConflict,
	"fork_child_cleanup_failed":         http.StatusBadGateway,
	"fork_child_turn_failed":            http.StatusBadGateway,
	"fork_job_not_found":                http.StatusNotFound,
	"fork_parent_restore_failed":        http.StatusConflict,
	"local_forget":                      http.StatusConflict,
	"local_status":                      http.StatusConflict,
	"missing_remote_session_id":         http.StatusConflict,
	"process_reap":                      http.StatusBadGateway,
	"process_reap_refs":                 http.StatusConflict,
	"remote_cancel":                     http.StatusBadGateway,
	"remote_close":                      http.StatusBadGateway,
	"remote_delete":                     http.StatusBadGateway,
	"remote_session_materialize_failed": http.StatusBadGateway,
	"run_related_session_retained":      http.StatusConflict,
}
