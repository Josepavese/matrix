package oslog

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestFailedCollectorRetainsPrimaryFileAndReportsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "primary.log")
	factory := NewFactory()
	sink, err := factory.Build(middleware.LogSinkOptions{Target: "file", FilePath: path, Collector: &middleware.CollectorOptions{Endpoint: server.URL, BatchSize: 1}})
	if err != nil {
		t.Fatal(err)
	}
	line := []byte(`{"event":"route_timing","msg":"PRIVATE_PRIMARY_ONLY","duration_ms":1}` + "\n")
	if n, err := sink.Writer().Write(line); err != nil || n != len(line) {
		t.Fatal("collector broke primary sink", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(line) {
		t.Fatal("primary log lost", err)
	}
	stats := factory.TelemetryStats()
	if stats.Acknowledged != 0 || stats.FailedBatches != 1 || strings.Contains(stats.LastFailure, "PRIVATE") {
		t.Fatal("collector failure not observed", stats)
	}
}

func TestDisabledCollectorCreatesNoExporter(t *testing.T) {
	factory := NewFactory()
	sink, err := factory.Build(middleware.LogSinkOptions{Target: "file", FilePath: filepath.Join(t.TempDir(), "local.log")})
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	if stats := factory.TelemetryStats(); stats.Enabled || factory.collector != nil {
		t.Fatal("disabled exporter was instantiated")
	}
}
