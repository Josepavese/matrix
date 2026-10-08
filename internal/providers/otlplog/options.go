package otlplog

import (
	"fmt"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

func normalizeOptions(options middleware.CollectorOptions) (middleware.CollectorOptions, error) {
	if options.QueueSize == 0 {
		options.QueueSize = 128
	}
	if options.BatchSize == 0 {
		options.BatchSize = min(16, options.QueueSize)
	}
	if options.FlushInterval == 0 {
		options.FlushInterval = 250 * time.Millisecond
	}
	if options.Timeout == 0 {
		options.Timeout = 2 * time.Second
	}
	if !validQueue(options) {
		return options, fmt.Errorf("invalid bounded collector queue configuration")
	}
	if !validTiming(options) {
		return options, fmt.Errorf("invalid collector timing configuration")
	}
	return options, nil
}

func validQueue(o middleware.CollectorOptions) bool {
	return o.QueueSize >= 1 && o.QueueSize <= 4096 && o.BatchSize >= 1 && o.BatchSize <= o.QueueSize
}
func validTiming(o middleware.CollectorOptions) bool {
	return o.Timeout > 0 && o.Timeout <= 30*time.Second && o.FlushInterval > 0 && o.FlushInterval <= 5*time.Second
}
