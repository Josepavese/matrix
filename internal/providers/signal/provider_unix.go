//go:build !windows

package signal

import (
	"os"
	"os/signal"
	"syscall"
)

// Wait blocks until os.Interrupt or SIGTERM is received.
func (p *Provider) Wait() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
}
