package otlplog

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestOTLPExportContainsOnlySelectedMetadataAndCountsPartialRejection(t *testing.T) {
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/logs" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "Bearer test-auth" {
			t.Error("invalid OTLP request")
		}
		body, _ := io.ReadAll(r.Body)
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"partialSuccess":{"rejectedLogRecords":"1","errorMessage":"never retain this text"}}`)
	}))
	defer server.Close()
	e, err := New(middleware.CollectorOptions{Endpoint: server.URL, BatchSize: 2, QueueSize: 2, Credential: func() (string, error) { return "Bearer test-auth", nil }})
	if err != nil {
		t.Fatal(err)
	}
	e.WriteJSON([]byte(`{"event":"route_completed","agent":"mimo","msg":"PRIVATE_TRANSCRIPT","prompt":"PRIVATE_PROMPT","api_key":"PRIVATE_SECRET","path":"/private/user/file"}`))
	e.WriteJSON([]byte(`{"event":"route_completed","status":"completed"}`))
	_ = e.Close()
	body := <-requests
	var wire payload
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Resources) != 1 || len(wire.Resources[0].Scopes[0].Records) != 2 {
		t.Fatal("invalid OTLP shape")
	}
	for _, forbidden := range []string{"PRIVATE_", "/private/user/file", "test-auth", "never retain"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatal("private content exported")
		}
	}
	stats := e.TelemetryStats()
	if stats.Acknowledged != 1 || stats.Dropped != 1 || stats.LastFailure != "collector_partial_rejection" {
		t.Fatal(stats)
	}
}

func TestCollectorRefusesInsecureCredentialURLsAndRedirects(t *testing.T) {
	for _, endpoint := range []string{"http://example.com/v1/logs", "https://user:secret@example.com", "https://example.com?token=secret", "file:///private"} {
		if _, err := New(middleware.CollectorOptions{Endpoint: endpoint}); err == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
	redirected := make(chan struct{}, 1)
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { redirected <- struct{}{}; _, _ = io.WriteString(w, `{}`) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer server.Close()
	e, err := New(middleware.CollectorOptions{Endpoint: server.URL, BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	e.WriteJSON([]byte(`{"event":"route_completed"}`))
	_ = e.Close()
	select {
	case <-redirected:
		t.Fatal("export followed a redirect")
	default:
	}
	if stats := e.TelemetryStats(); stats.Acknowledged != 0 || stats.FailedBatches != 1 || stats.LastFailure != "collector_http_302" {
		t.Fatal(stats)
	}
}

func TestSlowCollectorNeverBlocksProducerAndQueueIsBounded(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-entered:
		default:
			close(entered)
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	e, err := New(middleware.CollectorOptions{Endpoint: server.URL, QueueSize: 2, BatchSize: 1, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	e.WriteJSON([]byte(`{"event":"route_completed"}`))
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("collector never received a request")
	}
	for i := 0; i < 3; i++ {
		e.WriteJSON([]byte(`{"event":"route_completed"}`))
	}
	if stats := e.TelemetryStats(); stats.Queued != 2 || stats.Dropped != 1 {
		t.Fatal("queue was not bounded", stats)
	}
	e.WriteJSON([]byte(`{"msg":"private unstructured content"}`))
	if e.TelemetryStats().Filtered != 1 {
		t.Fatal("privacy filtering was not observed")
	}
	close(release)
	released = true
	_ = e.Close()
	if !e.TelemetryStats().Closed {
		t.Fatal("exporter did not close")
	}
}
