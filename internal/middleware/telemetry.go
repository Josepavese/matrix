package middleware

import "time"

// CollectorOptions describes an optional secondary telemetry destination.
// Credentials are resolved only by the provider and never retained in config views.
type CollectorOptions struct {
	Endpoint      string
	QueueSize     int
	BatchSize     int
	FlushInterval time.Duration
	Timeout       time.Duration
	Credential    func() (string, error) `json:"-"`
}

type TelemetryStats struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
	Acknowledged  uint64 `json:"acknowledged_records"`
	Dropped       uint64 `json:"dropped_records"`
	Filtered      uint64 `json:"filtered_records"`
	FailedBatches uint64 `json:"failed_batches"`
	Queued        int    `json:"queued_records"`
	LastFailure   string `json:"last_failure,omitempty"`
	Closed        bool   `json:"closed"`
}

type TelemetryReporter interface{ TelemetryStats() TelemetryStats }
