package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

func TestPALRuntimeRejectsUnauthorizedMutationAndConditionalStaleReads(t *testing.T) {
	store := memstore.New()
	runs := runtrace.NewStore(store)
	run, _, err := runs.Start(runtrace.Run{ID: "pal-http-proof", ClientMeta: map[string]interface{}{"private": "PRIVATE_SENTINEL"}})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerSemanticRuntime(mux, nil, store, "test-key")
	registerTelemetryRuntime(mux, func() middleware.TelemetryStats { return middleware.TelemetryStats{Enabled: true, Acknowledged: 2} }, "test-key")
	for _, path := range []string{"/_matrix/fs/", "/_matrix/telemetry"} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatal("runtime endpoint exposed", path, response.Code)
		}
	}
	for _, path := range []string{"/_matrix/fs/", "/_matrix/telemetry"} {
		request := httptest.NewRequest("POST", path, nil)
		request.Header.Set("Authorization", "Bearer test-key")
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatal("mutation accepted", path)
		}
	}
	if _, err := runs.Complete(run.ID, "PRIVATE_SUMMARY", "end_turn"); err != nil {
		t.Fatal(err)
	}
	path := "/_matrix/fs/runs/id-70616c2d687474702d70726f6f66/status.json"
	request := httptest.NewRequest("GET", path, nil)
	request.Header.Set("Authorization", "Bearer test-key")
	request.Header.Set("If-Modified-Since", time.Now().Add(time.Hour).Format(http.TimeFormat))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status": "completed"`) {
		t.Fatal("stale conditional projection", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "PRIVATE_") || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("privacy/cache contract")
	}
}
