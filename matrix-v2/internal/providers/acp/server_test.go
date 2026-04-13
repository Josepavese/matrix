package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jose/matrix-v2/internal/middleware"
)

type mockSessionRouter struct {
	lastChannelID string
	lastInput     string
	lastAgentID   string
	response      string
	err           error
	authResponse  string
	authErr       error
}

func (m *mockSessionRouter) Route(_ context.Context, channelID, agentID, input string, _ middleware.ThoughtNotifier) (string, error) {
	m.lastChannelID = channelID
	m.lastAgentID = agentID
	m.lastInput = input
	return m.response, m.err
}

func (m *mockSessionRouter) HandleAuthCallback(channelID, _, _ string) (string, error) {
	m.lastChannelID = channelID
	return m.authResponse, m.authErr
}

func setupServer(router *mockSessionRouter, apiKey string) (*Server, *http.ServeMux) {
	s := NewServer(router)
	if apiKey != "" {
		s.WithAPIKey(apiKey)
	}
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	return s, mux
}

func TestHandleRuns_MethodNotAllowed(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "")
	req := httptest.NewRequest(http.MethodGet, "/runs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleRuns_InvalidJSON(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "")
	req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRuns_MissingFields(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "")
	body, _ := json.Marshal(map[string]string{"channel_id": "ch1"})
	req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing input, got %d", w.Code)
	}
}

func TestHandleRuns_Unauthorized(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "secret-key")

	// No API key
	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "input": "hi"})
	req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	// Wrong API key
	req = httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(body))
	req.Header.Set("X-Matrix-Key", "wrong")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong key, got %d", w.Code)
	}
}

func TestHandleRuns_Success(t *testing.T) {
	router := &mockSessionRouter{response: "Hello from agent"}
	_, mux := setupServer(router, "secret-key")

	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "input": "hello", "agent_id": "gemini"})
	req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(body))
	req.Header.Set("X-Matrix-Key", "secret-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if router.lastChannelID != "ch1" {
		t.Errorf("expected channelID ch1, got %s", router.lastChannelID)
	}
	if router.lastAgentID != "gemini" {
		t.Errorf("expected agentID gemini, got %s", router.lastAgentID)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response parse error: %v", err)
	}
	if resp["output"] != "Hello from agent" {
		t.Errorf("unexpected output: %s", resp["output"])
	}
}

func TestHandleRuns_DefaultAgent(t *testing.T) {
	router := &mockSessionRouter{response: "ok"}
	_, mux := setupServer(router, "")

	body, _ := json.Marshal(map[string]string{"channel_id": "ch1", "input": "test"})
	req := httptest.NewRequest(http.MethodPost, "/runs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if router.lastAgentID != "opencode" {
		t.Errorf("expected default agent 'opencode', got %s", router.lastAgentID)
	}
}

func TestHandleOpenRouterCallback_MissingParams(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "")

	req := httptest.NewRequest(http.MethodGet, "/auth/openrouter/callback", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleOpenRouterCallback_MethodNotAllowed(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "")
	req := httptest.NewRequest(http.MethodPost, "/auth/openrouter/callback?code=x&state=y", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleOpenRouterCallback_Success(t *testing.T) {
	router := &mockSessionRouter{authResponse: "Auth OK"}
	_, mux := setupServer(router, "")

	req := httptest.NewRequest(http.MethodGet, "/auth/openrouter/callback?code=abc123&state=ch1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Auth OK")) {
		t.Errorf("response should contain auth result: %s", w.Body.String())
	}
}
