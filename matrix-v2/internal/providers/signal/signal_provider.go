package signal

import (
	"os"
	"os/signal"
	"syscall"
)

// Provider implements middleware.Signal using standard os/signal
type Provider struct{}

// NewProvider returns a new signal provider
func NewProvider() *Provider {
	return &Provider{}
}

// Wait blocks until os.Interrupt or syscall.SIGTERM (on Unix) is received
func (p *Provider) Wait() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
}
