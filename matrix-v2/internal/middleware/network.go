package middleware

import (
	"context"
	"net"
)

// Network defines the network abstraction layer
type Network interface {
	// Listen opens a primary listener for the daemon
	Listen(network, address string) (ClosableListener, error)
	// Download fetches a file from a URL and saves it to a local path
	Download(ctx context.Context, url, destPath string) error
	// FetchJSON fetches a JSON resource from a URL and decodes it into the target
	FetchJSON(ctx context.Context, url string, target interface{}) error
	GetFreePort() (int, error)
	// Fetch fetches a URL and returns the response body bytes
	Fetch(ctx context.Context, url string) ([]byte, error)
	// PostJSON posts a JSON body to a URL and returns the response body bytes
	PostJSON(ctx context.Context, url string, body interface{}) ([]byte, int, error)
	// CanDial reports whether a TCP address is reachable
	CanDial(address string) bool
}

// ClosableListener is a minimal interface for a network listener
type ClosableListener interface {
	Accept() (net.Conn, error)
	Close() error
	Addr() net.Addr
}
