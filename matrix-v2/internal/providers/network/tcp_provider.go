package network

import (
	"net"

	"github.com/jose/matrix-v2/internal/middleware"
)

// Provider implements middleware.Network using standard TCP
type Provider struct{}

// NewProvider returns a new TCP network provider
func NewProvider() *Provider {
	return &Provider{}
}

// Listen opens a standard net.Listen socket
func (p *Provider) Listen(network, address string) (middleware.ClosableListener, error) {
	return net.Listen(network, address)
}
