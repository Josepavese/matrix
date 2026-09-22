package matrixapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenRouterCallbackEscapesTheRouterMessage is defence in depth on a page
// served from the local origin: the message comes from a router implementation,
// and any markup in it would execute in the operator's browser.
func TestOpenRouterCallbackEscapesTheRouterMessage(t *testing.T) {
	router := &mockSessionRouter{authResponse: `<script>alert("xss")</script>`}
	_, mux := setupServer(router, "", "")

	req := newJSONRequest(http.MethodGet, OpenRouterCallbackV1+"?code=abc123&state=ch1", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	body := recorder.Body.String()
	if bytes.Contains(recorder.Body.Bytes(), []byte("<script>")) {
		t.Fatalf("the router message must not be rendered as markup: %s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("the message must still be readable, escaped: %s", body)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("unexpected content type %q", contentType)
	}
}
