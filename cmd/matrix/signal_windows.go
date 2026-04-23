//go:build windows

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
)

// signalContext returns a context that is cancelled on Ctrl+C on Windows.
func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	go func() {
		defer signal.Stop(signals)
		select {
		case sig := <-signals:
			slog.Info("runtime signal received", "component", "runtime", "event", "runtime_signal_received", "signal", sig.String())
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}
