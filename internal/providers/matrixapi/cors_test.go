package matrixapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalCORSHandlerAllowsLoopbackPreflight(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "", "")
	handler := LocalCORSHandler(mux)
	req := httptest.NewRequest(http.MethodOptions, RunPathV1, nil)
	req.Header.Set("Origin", "http://127.0.0.1:30031")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:30031" {
		t.Fatalf("unexpected allow origin: %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") || !strings.Contains(got, "OPTIONS") {
		t.Fatalf("unexpected allow methods: %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Content-Type") || !strings.Contains(got, "X-Matrix-Key") || !strings.Contains(got, "Authorization") {
		t.Fatalf("unexpected allow headers: %q", got)
	}
	if got := w.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("expected Vary Origin, got %q", got)
	}
}

func TestLocalCORSHandlerRejectsRemoteOrigin(t *testing.T) {
	_, mux := setupServer(&mockSessionRouter{}, "", "")
	handler := LocalCORSHandler(mux)
	req := httptest.NewRequest(http.MethodOptions, RunPathV1, nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("remote origin should not get CORS allow header, got %q", got)
	}
}

func TestLocalCORSHandlerAddsHeadersToLoopbackPost(t *testing.T) {
	router := &mockSessionRouter{response: "OK"}
	_, mux := setupServer(router, "", "")
	handler := LocalCORSHandler(mux)
	body, _ := json.Marshal(map[string]interface{}{
		"channel_id": "halfdesk.pm.smoke",
		"agent_id":   "codex",
		"input": map[string]string{
			"text": "Rispondi solo OK.",
		},
	})
	req := newJSONRequest(http.MethodPost, RunPathV1, bytes.NewReader(body))
	req.Header.Set("Origin", "http://localhost:30031")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:30031" {
		t.Fatalf("unexpected allow origin: %q", got)
	}
	if router.lastChannelID != "halfdesk.pm.smoke" || router.lastAgentID != "codex" {
		t.Fatalf("request did not reach router with expected route: channel=%q agent=%q", router.lastChannelID, router.lastAgentID)
	}
}

func TestIsLoopbackHTTPOrigin(t *testing.T) {
	tests := []struct {
		origin string
		want   bool
	}{
		{origin: "http://localhost:30031", want: true},
		{origin: "http://127.0.0.1:30031", want: true},
		{origin: "http://[::1]:30031", want: true},
		{origin: "https://example.com", want: false},
		{origin: "http://127.0.0.1.example.com", want: false},
		{origin: "https://localhost:30031", want: false},
		{origin: "http://localhost:30031/path", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			if got := isLoopbackHTTPOrigin(tt.origin); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
