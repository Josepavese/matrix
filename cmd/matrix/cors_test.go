package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalCORSMiddlewareAllowsLocalPreflight(t *testing.T) {
	handler := localCORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight should not reach next handler")
	}))
	req := httptest.NewRequest(http.MethodOptions, "/v1/runs", nil)
	req.Header.Set("Origin", "http://127.0.0.1:30031")
	req.Header.Set("Access-Control-Request-Method", "POST")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:30031" {
		t.Fatalf("unexpected allow origin %q", got)
	}
}

func TestLocalCORSMiddlewareRejectsRemotePreflight(t *testing.T) {
	handler := localCORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("rejected preflight should not reach next handler")
	}))
	req := httptest.NewRequest(http.MethodOptions, "/v1/runs", nil)
	req.Header.Set("Origin", "https://example.com")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestLocalCORSMiddlewarePassesRequestsWithoutOrigin(t *testing.T) {
	called := false
	handler := localCORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/runs", nil)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("expected next handler to be called")
	}
	if w.Code != http.StatusTeapot {
		t.Fatalf("expected downstream status, got %d", w.Code)
	}
}
