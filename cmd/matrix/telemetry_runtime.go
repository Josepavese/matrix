package main

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Josepavese/matrix/internal/middleware"
)

func registerTelemetryRuntime(mux *http.ServeMux, stats func() middleware.TelemetryStats, key string) {
	mux.HandleFunc("/_matrix/telemetry", func(w http.ResponseWriter, r *http.Request) {
		if !authorizeMatrixRuntimeRequest(w, r, key) {
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(stats()); err != nil {
			slog.Warn("telemetry report encoding failed", "event", "telemetry_report_encode_failed", "error", err)
		}
	})
}
