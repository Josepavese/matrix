// Package signal provides a cross-platform signal handling provider using os/signal.
package signal

// Provider implements middleware.Signal using standard os/signal.
type Provider struct{}

// NewProvider returns a new signal provider.
func NewProvider() *Provider {
	return &Provider{}
}
