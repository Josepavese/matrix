package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jose/matrix-v2/internal/middleware"
)

// SessionRouter abstracts the logic/session routing capability.
type SessionRouter interface {
	Route(ctx context.Context, channelID string, agentID string, input string, notifier middleware.ThoughtNotifier) (string, error)
	HandleAuthCallback(channelID, provider, code string) (string, error)
}

// Server provides the RESTful ACP endpoints over HTTP.
type Server struct {
	router SessionRouter
	apiKey string
}

// NewServer creates a new ACP Server provider.
func NewServer(router SessionRouter) *Server {
	return &Server{
		router: router,
	}
}

// WithAPIKey sets an optional API key for authenticating requests.
func (s *Server) WithAPIKey(key string) *Server {
	s.apiKey = key
	return s
}

// payload POST /runs
// In ACP a Run payload determines who to talk to via the Authorization header or specific body params.
// We use a simplified form here where the "channel_id" comes in the body.
type runRequest struct {
	ChannelID string `json:"channel_id"` // E.g., telegram.user123 (routing key)
	Input     string `json:"input"`
	AgentID   string `json:"agent_id"` // Target agent (e.g. opencode, gemini, claude)
}

// HandleOpenRouterCallback handles GET /auth/openrouter/callback
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

// HandleRuns is the HTTP handler for POST /runs
func (s *Server) HandleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// API key validation
	if s.apiKey != "" {
		if r.Header.Get("X-Matrix-Key") != s.apiKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	var req runRequest
	// In the real ACP protocol we would parse the full manifest.
	// For MVP Session Routing we take the core channel + messages.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
		return
	}

	if req.ChannelID == "" || req.Input == "" {
		http.Error(w, "Bad Request: channel_id and input are required", http.StatusBadRequest)
		return
	}

	// We take the first message for our simple MVP routing.
	// Multi-message conversations are possible but for simplicity we wrap the latest.
	agentID := req.AgentID
	if agentID == "" {
		agentID = "opencode" // default
	}
	res, err := s.router.Route(r.Context(), req.ChannelID, agentID, req.Input, nil)
	if err != nil {
		// Distinguish HTTP 500
		slog.Error("ACP run failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]string{"output": res}); err != nil {
		slog.Error("ACP server failed to encode response", "error", err)
	}
}

// RegisterRoutes attaches the ACP REST endpoints to the provided HTTP ServeMux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/runs", s.HandleRuns)
	mux.HandleFunc("/auth/openrouter/callback", s.HandleOpenRouterCallback)
}
