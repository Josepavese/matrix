package logging

import (
	"fmt"
	"strconv"

	"github.com/Josepavese/matrix/internal/logic/config"
	"github.com/Josepavese/matrix/internal/middleware"
)

func loadCollectorConfig(manager *config.Manager, cfg *Config) error {
	if manager == nil {
		return nil
	}
	endpoint, err := manager.Get("system.logging.collector.endpoint")
	if err != nil {
		return fmt.Errorf("collector configuration unavailable")
	}
	if endpoint == "" {
		return nil
	}
	if cfg.Format != "json" {
		return fmt.Errorf("collector export requires JSON logging")
	}
	raw, err := manager.Get("system.logging.collector.queue_size")
	if err != nil {
		return fmt.Errorf("collector configuration unavailable")
	}
	if raw == "" {
		raw = "128"
	}
	size, err := strconv.Atoi(raw)
	if err != nil || size < 16 || size > 4096 {
		return fmt.Errorf("collector queue_size must be between 16 and 4096")
	}
	cfg.CollectorEndpoint = endpoint
	cfg.CollectorQueueSize = size
	return nil
}

func telemetryReporter(factory middleware.LogSinkFactory) func() middleware.TelemetryStats {
	if reporter, ok := factory.(middleware.TelemetryReporter); ok {
		return reporter.TelemetryStats
	}
	return func() middleware.TelemetryStats { return middleware.TelemetryStats{} }
}

func collectorOptions(cfg Config) *middleware.CollectorOptions {
	if cfg.CollectorEndpoint == "" {
		return nil
	}
	return &middleware.CollectorOptions{Endpoint: cfg.CollectorEndpoint, QueueSize: cfg.CollectorQueueSize}
}
