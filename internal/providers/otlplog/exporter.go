// Package otlplog exports a bounded subset of Matrix operational log metadata
// as OTLP/HTTP JSON. It never exports the source message or arbitrary attributes.
package otlplog

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

type Exporter struct {
	mu            sync.Mutex
	options       middleware.CollectorOptions
	client        *http.Client
	authorization string
	queue         chan record
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
	closeOnce     sync.Once
	stats         middleware.TelemetryStats
}

func New(options middleware.CollectorOptions) (*Exporter, error) {
	endpoint, err := validateEndpoint(options.Endpoint)
	if err != nil {
		return nil, err
	}
	options, err = normalizeOptions(options)
	if err != nil {
		return nil, err
	}
	authorization, err := resolveAuthorization(options.Credential)
	if err != nil {
		return nil, err
	}
	options.Endpoint = endpoint
	options.Credential = nil
	ctx, cancel := context.WithCancel(context.Background())
	e := &Exporter{options: options, authorization: authorization, queue: make(chan record, options.QueueSize), ctx: ctx, cancel: cancel, done: make(chan struct{}), stats: middleware.TelemetryStats{Enabled: true, Endpoint: endpoint, Protocol: "otlp/http-json"}}
	e.client = &http.Client{Timeout: options.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	go e.run()
	return e, nil
}

func (e *Exporter) WriteJSON(raw []byte) {
	item, ok := sanitize(raw)
	if !ok {
		e.mu.Lock()
		e.stats.Filtered++
		e.mu.Unlock()
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stats.Closed {
		e.stats.Dropped++
		return
	}
	select {
	case e.queue <- item:
	default:
		e.stats.Dropped++
	}
}

func (e *Exporter) drop(count uint64) { e.mu.Lock(); e.stats.Dropped += count; e.mu.Unlock() }

func (e *Exporter) TelemetryStats() middleware.TelemetryStats {
	e.mu.Lock()
	defer e.mu.Unlock()
	stats := e.stats
	stats.Queued = len(e.queue)
	return stats
}

// Close drains only within the explicitly bounded shutdown window. It never
// blocks the runtime indefinitely on an unavailable collector.
func (e *Exporter) Close() error {
	e.closeOnce.Do(func() { e.mu.Lock(); e.stats.Closed = true; close(e.queue); e.mu.Unlock() })
	timer := time.NewTimer(min(5*time.Second, 2*e.options.Timeout+e.options.FlushInterval))
	defer timer.Stop()
	select {
	case <-e.done:
	case <-timer.C:
		e.cancel()
		<-e.done
	}
	e.cancel()
	e.client.CloseIdleConnections()
	return nil
}

func (e *Exporter) run() {
	defer close(e.done)
	ticker := time.NewTicker(e.options.FlushInterval)
	defer ticker.Stop()
	batch := make([]record, 0, e.options.BatchSize)
	for {
		select {
		case <-e.ctx.Done():
			e.drop(uint64(len(batch) + len(e.queue)))
			return
		case item, ok := <-e.queue:
			if !ok {
				if len(batch) > 0 {
					e.send(batch)
				}
				return
			}
			batch = append(batch, item)
			if len(batch) >= e.options.BatchSize {
				e.send(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				e.send(batch)
				batch = batch[:0]
			}
		}
	}
}
