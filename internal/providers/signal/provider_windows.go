//go:build windows

package signal

import (
	"os"
	"os/signal"
)

// Wait blocks until Ctrl+C is received on Windows.
func (p *Provider) Wait() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}
