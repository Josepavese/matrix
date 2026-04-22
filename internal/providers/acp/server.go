// Package acp exposes Matrix's inbound HTTP bridge for channel/session routing.
// This is not the Zed ACP wire protocol server surface; Matrix speaks Zed ACP
// outbound to agents over JSON-RPC transports and exposes /v1/runs as a Matrix API.
package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jose/matrix-v2/internal/logic/orchestration"
	"github.com/jose/matrix-v2/internal/middleware"
	"github.com/jose/matrix-v2/internal/providers/runapi"
)

// SessionRouter abstracts the logic/session routing capability.
type SessionRouter interface {
	middleware.SessionRouter
	middleware.AuthCallbackHandler
}

// Server provides Matrix's HTTP routing endpoints over HTTP.
type Server struct {
	router       SessionRouter
	apiKey       string
	defaultAgent string
	runs         *runapi.Server
}

// NewServer creates a new ACP Server provider.
func NewServer(router SessionRouter) *Server {
	return &Server{
		router:       router,
		defaultAgent: "opencode",
		runs:         runapi.NewServer(router),
	}
}

// WithAPIKey sets an optional API key for authenticating requests.
func (s *Server) WithAPIKey(key string) *Server {
	s.apiKey = key
	s.runs.WithAPIKey(key)
	return s
}

// WithDefaultAgent sets the default agent used when agent_id is omitted.
func (s *Server) WithDefaultAgent(agentID string) *Server {
	if agentID != "" {
		s.defaultAgent = agentID
		s.runs.WithDefaultAgent(agentID)
	}
	return s
}

// WithTraceStorage wires Matrix run traces to the canonical vault storage.
func (s *Server) WithTraceStorage(storage middleware.Storage) *Server {
	s.runs.WithTraceStorage(storage)
	return s
}

// WithEndpointResolver lets run traces record the selected protocol family.
func (s *Server) WithEndpointResolver(resolver middleware.AgentEndpointResolver) *Server {
	s.runs.WithEndpointResolver(resolver)
	return s
}

func (s *Server) StartRunSinkDeliveryWorker(ctx context.Context) {
	s.runs.StartSinkDeliveryWorker(ctx)
}

const (
	RunPathV1                = runapi.RunPathV1
	RunResourcePrefixV1      = runapi.RunResourcePrefixV1
	EventSinksPathV1         = runapi.EventSinksPathV1
	SessionActionPathV1      = "/v1/session-actions"
	WorkspaceActionPathV1    = "/v1/workspace-actions"
	WorkspaceStatePathV1     = "/v1/workspace-state"
	WorkspaceTimelinePathV1  = "/v1/workspace-timeline"
	WorkspaceDecisionsPathV1 = "/v1/workspace-decisions"
	WorkspaceMemoryPathV1    = "/v1/workspace-memory"
	WorkspaceSnapshotsPathV1 = "/v1/workspace-snapshots"
	IntentActionPathV1       = "/v1/intents"
	ModeActionPathV1         = "/v1/modes"
	OrchestrationProfileV1   = "/v1/orchestration-capabilities"
	OpenRouterCallbackV1     = "/v1/auth/openrouter/callback"
)

// payload POST /v1/session-actions
// Target semantics depend on action:
// - cancel/delete/switch: local or remote session selector
// - new: agent id
// - name: alias
type sessionActionRequest struct {
	ChannelID        string `json:"channel_id"`
	Action           string `json:"action"`
	Target           string `json:"target,omitempty"`
	WorkspaceID      string `json:"workspace_id,omitempty"`
	WorkspacePath    string `json:"workspace_path,omitempty"`
	Ephemeral        bool   `json:"ephemeral,omitempty"`
	CleanupPolicy    string `json:"cleanup_policy,omitempty"`
	ForceForgetLocal bool   `json:"force_forget_local,omitempty"`
	MakeActive       *bool  `json:"make_active,omitempty"`
	RestoreParent    bool   `json:"restore_parent,omitempty"`
	Input            string `json:"input,omitempty"`
}

type workspaceActionRequest struct {
	ChannelID string `json:"channel_id"`
	Action    string `json:"action"`
	Target    string `json:"target,omitempty"`
}

type intentActionRequest struct {
	ChannelID   string `json:"channel_id"`
	Intent      string `json:"intent"`
	Target      string `json:"target,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	Note        string `json:"note,omitempty"`
}

type modeActionRequest struct {
	ChannelID string `json:"channel_id"`
	Mode      string `json:"mode"`
	Target    string `json:"target,omitempty"`
}

// HandleOpenRouterCallback handles GET /v1/auth/openrouter/callback.
func (s *Server) HandleOpenRouterCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state") // channelID

	if code == "" || state == "" {
		http.Error(w, "Missing code or state", http.StatusBadRequest)
		return
	}

	res, err := s.router.HandleAuthCallback(state, "openrouter", code)
	if err != nil {
		slog.Error("auth callback failed", "error", err)
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprintf(w, "<html><body><h1>%s</h1><p>You can close this window and go back to Telegram.</p></body></html>", res)
}

// HandleSessionActions is the typed HTTP handler for session lifecycle actions.
func (s *Server) HandleSessionActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" {
		if r.Header.Get("X-Matrix-Key") != s.apiKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	var req sessionActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" || req.Action == "" {
		http.Error(w, "Bad Request: channel_id and action are required", http.StatusBadRequest)
		return
	}

	result, err := s.router.HandleSessionActionTyped(r.Context(), middleware.SessionActionRequest{
		ChannelID:        req.ChannelID,
		Action:           req.Action,
		Target:           req.Target,
		WorkspaceID:      req.WorkspaceID,
		WorkspacePath:    req.WorkspacePath,
		Ephemeral:        req.Ephemeral,
		CleanupPolicy:    req.CleanupPolicy,
		ForceForgetLocal: req.ForceForgetLocal,
		MakeActive:       req.MakeActive,
		RestoreParent:    req.RestoreParent,
		Input:            req.Input,
	})
	if err != nil {
		slog.Error("matrix session action failed", "error", err, "action", req.Action)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

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
	switch result.Error.Code {
	case "agent_not_found":
		return http.StatusNotFound
	case "missing_remote_session_id":
		return http.StatusConflict
	case "remote_session_materialize_failed":
		return http.StatusBadGateway
	default:
		return http.StatusBadRequest
	}
}

// HandleWorkspaceActions is the typed HTTP handler for workspace control actions.
func (s *Server) HandleWorkspaceActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" && r.Header.Get("X-Matrix-Key") != s.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var req workspaceActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" || req.Action == "" {
		http.Error(w, "Bad Request: channel_id and action are required", http.StatusBadRequest)
		return
	}
	result, err := s.router.HandleWorkspaceActionTyped(r.Context(), middleware.WorkspaceActionRequest{
		ChannelID: req.ChannelID,
		Action:    req.Action,
		Target:    req.Target,
	})
	if err != nil {
		slog.Error("matrix workspace action failed", "error", err, "action", req.Action)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("matrix workspace action failed to encode response", "error", err)
	}
}

// HandleWorkspaceState is the typed HTTP handler for current workspace state.
func (s *Server) HandleWorkspaceState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" && r.Header.Get("X-Matrix-Key") != s.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	channelID := r.URL.Query().Get("channel_id")
	workspaceID := r.URL.Query().Get("workspace_id")
	if channelID == "" && workspaceID == "" {
		http.Error(w, "Bad Request: channel_id or workspace_id is required", http.StatusBadRequest)
		return
	}
	result, err := s.router.HandleWorkspaceReadTyped(r.Context(), middleware.WorkspaceReadRequest{
		ChannelID:   channelID,
		Action:      "state",
		WorkspaceID: workspaceID,
	})
	if err != nil {
		slog.Error("matrix workspace state failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("matrix workspace state failed to encode response", "error", err)
	}
}

// HandleWorkspaceTimeline is the typed HTTP handler for workspace timeline reads.
func (s *Server) HandleWorkspaceTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" && r.Header.Get("X-Matrix-Key") != s.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	channelID := r.URL.Query().Get("channel_id")
	workspaceID := r.URL.Query().Get("workspace_id")
	limit := 10
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if channelID == "" && workspaceID == "" {
		http.Error(w, "Bad Request: channel_id or workspace_id is required", http.StatusBadRequest)
		return
	}
	result, err := s.router.HandleWorkspaceReadTyped(r.Context(), middleware.WorkspaceReadRequest{
		ChannelID:   channelID,
		Action:      "timeline",
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
	if err != nil {
		slog.Error("matrix workspace timeline failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("matrix workspace timeline failed to encode response", "error", err)
	}
}

// HandleWorkspaceDecisions is the typed HTTP handler for workspace decision-trace reads.
func (s *Server) HandleWorkspaceDecisions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" && r.Header.Get("X-Matrix-Key") != s.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	channelID := r.URL.Query().Get("channel_id")
	workspaceID := r.URL.Query().Get("workspace_id")
	limit := 10
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if channelID == "" && workspaceID == "" {
		http.Error(w, "Bad Request: channel_id or workspace_id is required", http.StatusBadRequest)
		return
	}
	result, err := s.router.HandleWorkspaceReadTyped(r.Context(), middleware.WorkspaceReadRequest{
		ChannelID:   channelID,
		Action:      "decisions",
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
	if err != nil {
		slog.Error("matrix workspace decisions failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("matrix workspace decisions failed to encode response", "error", err)
	}
}

// HandleWorkspaceMemory is the typed HTTP handler for workspace memory reads.
func (s *Server) HandleWorkspaceMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" && r.Header.Get("X-Matrix-Key") != s.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	channelID := r.URL.Query().Get("channel_id")
	workspaceID := r.URL.Query().Get("workspace_id")
	limit := 12
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if channelID == "" && workspaceID == "" {
		http.Error(w, "Bad Request: channel_id or workspace_id is required", http.StatusBadRequest)
		return
	}
	result, err := s.router.HandleWorkspaceReadTyped(r.Context(), middleware.WorkspaceReadRequest{
		ChannelID:   channelID,
		Action:      "memory",
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
	if err != nil {
		slog.Error("matrix workspace memory failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("matrix workspace memory failed to encode response", "error", err)
	}
}

// HandleWorkspaceSnapshots is the typed HTTP handler for workspace snapshot reads.
func (s *Server) HandleWorkspaceSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" && r.Header.Get("X-Matrix-Key") != s.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	channelID := r.URL.Query().Get("channel_id")
	workspaceID := r.URL.Query().Get("workspace_id")
	limit := 10
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if channelID == "" && workspaceID == "" {
		http.Error(w, "Bad Request: channel_id or workspace_id is required", http.StatusBadRequest)
		return
	}
	result, err := s.router.HandleWorkspaceReadTyped(r.Context(), middleware.WorkspaceReadRequest{
		ChannelID:   channelID,
		Action:      "snapshots",
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
	if err != nil {
		slog.Error("matrix workspace snapshots failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("matrix workspace snapshots failed to encode response", "error", err)
	}
}

// HandleIntents is the typed HTTP handler for high-level operator intents.
func (s *Server) HandleIntents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" && r.Header.Get("X-Matrix-Key") != s.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var req intentActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" || req.Intent == "" {
		http.Error(w, "Bad Request: channel_id and intent are required", http.StatusBadRequest)
		return
	}
	result, err := s.router.HandleIntentTyped(r.Context(), middleware.IntentActionRequest{
		ChannelID:   req.ChannelID,
		Intent:      req.Intent,
		Target:      req.Target,
		WorkspaceID: req.WorkspaceID,
		AgentID:     req.AgentID,
		Note:        req.Note,
	})
	if err != nil {
		slog.Error("matrix intent failed", "error", err, "intent", req.Intent)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("matrix intent failed to encode response", "error", err)
	}
}

// HandleModes is the typed HTTP handler for explicit work-mode switches.
func (s *Server) HandleModes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" && r.Header.Get("X-Matrix-Key") != s.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var req modeActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" || req.Mode == "" {
		http.Error(w, "Bad Request: channel_id and mode are required", http.StatusBadRequest)
		return
	}
	result, err := s.router.HandleIntentTyped(r.Context(), middleware.IntentActionRequest{
		ChannelID: req.ChannelID,
		Intent:    req.Mode,
		Target:    req.Target,
	})
	if err != nil {
		slog.Error("matrix mode switch failed", "error", err, "mode", req.Mode)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("matrix mode switch failed to encode response", "error", err)
	}
}

// HandleOrchestrationCapabilities returns Matrix's machine-readable orchestration profile.
func (s *Server) HandleOrchestrationCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" && r.Header.Get("X-Matrix-Key") != s.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(orchestration.ProfileV1()); err != nil {
		slog.Error("matrix orchestration profile failed to encode response", "error", err)
	}
}

// RegisterRoutes attaches Matrix's inbound HTTP endpoints to the provided mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	s.runs.RegisterRoutes(mux)
	mux.HandleFunc(SessionActionPathV1, s.HandleSessionActions)
	mux.HandleFunc(WorkspaceActionPathV1, s.HandleWorkspaceActions)
	mux.HandleFunc(WorkspaceStatePathV1, s.HandleWorkspaceState)
	mux.HandleFunc(WorkspaceTimelinePathV1, s.HandleWorkspaceTimeline)
	mux.HandleFunc(WorkspaceDecisionsPathV1, s.HandleWorkspaceDecisions)
	mux.HandleFunc(WorkspaceMemoryPathV1, s.HandleWorkspaceMemory)
	mux.HandleFunc(WorkspaceSnapshotsPathV1, s.HandleWorkspaceSnapshots)
	mux.HandleFunc(IntentActionPathV1, s.HandleIntents)
	mux.HandleFunc(ModeActionPathV1, s.HandleModes)
	mux.HandleFunc(OrchestrationProfileV1, s.HandleOrchestrationCapabilities)
	mux.HandleFunc(OpenRouterCallbackV1, s.HandleOpenRouterCallback)
}
