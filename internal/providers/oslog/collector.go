package oslog

import (
	"io"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/otlplog"
)

// WithCollectorCredential is configured before Build; no credential value is
// retained in public logging settings or descriptions.
func (f *Factory) WithCollectorCredential(resolve func() (string, error)) *Factory {
	f.credential = resolve
	return f
}

func (f *Factory) TelemetryStats() middleware.TelemetryStats {
	f.mu.Lock()
	exporter := f.collector
	f.mu.Unlock()
	if exporter == nil {
		return middleware.TelemetryStats{}
	}
	return exporter.TelemetryStats()
}

type collectorSink struct {
	primary  middleware.LogSink
	exporter *otlplog.Exporter
}

func (s *collectorSink) Writer() io.Writer  { return s }
func (s *collectorSink) Descriptor() string { return s.primary.Descriptor() + "+otlp" }
func (s *collectorSink) Write(p []byte) (int, error) {
	n, err := s.primary.Writer().Write(p)
	if err == nil && n == len(p) {
		s.exporter.WriteJSON(p)
	}
	return n, err
}
func (s *collectorSink) Close() error {
	_ = s.exporter.Close()
	return s.primary.Close()
}
