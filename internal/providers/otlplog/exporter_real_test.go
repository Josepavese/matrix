package otlplog

import (
	"os"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestSmokeOfficialCollectorAcceptsOperationalMetadata(t *testing.T) {
	endpoint := os.Getenv("MATRIX_REAL_OTLP_ENDPOINT")
	if endpoint == "" {
		t.Skip("set MATRIX_REAL_OTLP_ENDPOINT for a real collector probe")
	}
	e, err := New(middleware.CollectorOptions{Endpoint: endpoint, BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	e.WriteJSON([]byte(`{"event":"matrix_pal_probe","component":"matrix","duration_ms":7,"prompt":"MATRIX_PRIVATE_PROBE_MUST_NOT_EXPORT"}`))
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	stats := e.TelemetryStats()
	if stats.Acknowledged != 1 || stats.Dropped != 0 || stats.FailedBatches != 0 {
		t.Fatal(stats)
	}
	t.Log("Official collector acknowledged OTLP/HTTP JSON metadata; no private input was selected")
}
