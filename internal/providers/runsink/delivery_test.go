package runsink

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
)

func TestPostRejectsLocalSinkTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("local sink should not be called")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	err := Post(runtrace.Sink{URL: server.URL}, runtrace.Event{ID: "evt", RunID: "run", Kind: "run.started"})
	if err == nil || !strings.Contains(err.Error(), "sink url must not target") {
		t.Fatalf("expected local sink rejection, got %v", err)
	}
}

func TestSafeSinkHTTPClientRejectsRedirectToLocalTarget(t *testing.T) {
	client := safeSinkHTTPClient()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/events", nil)

	err := client.CheckRedirect(req, []*http.Request{{}})
	if err == nil || !strings.Contains(err.Error(), "sink url must not target") {
		t.Fatalf("expected redirect target rejection, got %v", err)
	}
}
