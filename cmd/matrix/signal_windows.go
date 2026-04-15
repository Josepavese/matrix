//go:build windows

package main

import (
	"context"
	"os"
	"os/signal"
)

// signalContext returns a context that is cancelled on Ctrl+C on Windows.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}
