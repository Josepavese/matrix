package middleware

import "net"

// Network defines the network abstraction layer
type Network interface {
	// Listen opens a primary listener for the daemon
	Listen(network, address string) (ClosableListener, error)
}

// ClosableListener is a minimal interface for a network listener
type ClosableListener interface {
	Accept() (net.Conn, error)
	Close() error
	Addr() net.Addr
}
