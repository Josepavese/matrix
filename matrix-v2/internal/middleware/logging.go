package middleware

import "io"

// LogSinkOptions configures the destination and rotation parameters for a log sink.
type LogSinkOptions struct {
	Target     string
	FilePath   string
	MaxBytes   int64
	MaxBackups int
}

// LogSink represents a concrete runtime logging destination.
// Implementations are provider-owned and cross-platform.
type LogSink interface {
	Writer() io.Writer
	Close() error
	Descriptor() string
}

// LogSinkFactory creates runtime logging sinks from high-level configuration.
type LogSinkFactory interface {
	Build(options LogSinkOptions) (LogSink, error)
}
