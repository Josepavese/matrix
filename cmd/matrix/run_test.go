package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorizeMatrixRuntimeRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/_matrix/runtime", nil)
	w := httptest.NewRecorder()
	if authorizeMatrixRuntimeRequest(w, req, "secret") {
		t.Fatal("expected runtime request without API key to be rejected")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/_matrix/runtime", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	if !authorizeMatrixRuntimeRequest(w, req, "secret") {
		t.Fatal("expected bearer-authenticated runtime request to be accepted")
	}
}
