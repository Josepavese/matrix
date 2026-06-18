// Package daemon implements the JSON-RPC daemon server.
package daemon

import (
	"context"
	"log/slog"
	"net/rpc"
	"net/rpc/jsonrpc"
	"sync"

	"github.com/Josepavese/matrix/internal/logic/vault"
	"github.com/Josepavese/matrix/internal/middleware"
)

// Server represents the JSON-RPC Daemon server
type Server struct {
	rpcServer *rpc.Server
	net       middleware.Network
	mu        sync.Mutex
	listener  middleware.ClosableListener
	vault     *vault.Vault
	apiKey    string
}

// NewServer initializes a new JSON-RPC Daemon
func NewServer(v *vault.Vault, n middleware.Network) *Server {
	return &Server{
		rpcServer: rpc.NewServer(),
		vault:     v,
		net:       n,
	}
}

// WithAPIKey enables API key authentication for the JSON-RPC daemon.
func (s *Server) WithAPIKey(key string) *Server {
	s.apiKey = key
	return s
}

// Start opens the TCP socket and serves JSON-RPC requests until the context is cancelled.
// Returns nil on graceful shutdown, or an error if startup fails.
func (s *Server) Start(ctx context.Context, addr string) error {
	log := slog.With("component", "daemon")

	vaultSvc := NewVaultService(s.vault, s.apiKey)
	if err := s.rpcServer.RegisterName("Vault", vaultSvc); err != nil {
		return err
	}

	if s.apiKey != "" {
		authSvc := &AuthService{apiKey: s.apiKey}
		if err := s.rpcServer.RegisterName("Auth", authSvc); err != nil {
			return err
		}
		log.Info("daemon API key authentication enabled", "event", "daemon_auth_enabled")
	}

	l, err := s.net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = l
	s.mu.Unlock()

	log.Info("matrix daemon started", "event", "daemon_started", "addr", addr, "protocol", "json-rpc")

	// Close the listener when context is cancelled, which unblocks Accept.
	go func() {
		<-ctx.Done()
		log.Info("daemon context cancelled, closing listener", "event", "daemon_shutdown")
		s.mu.Lock()
		if s.listener != nil {
			_ = s.listener.Close()
		}
		s.mu.Unlock()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			// Check if we're shutting down
			select {
			case <-ctx.Done():
				log.Info("daemon stopped gracefully", "event", "daemon_stopped")
				return nil
			default:
				log.Error("accept error", "event", "accept_failed", "error", err)
				continue
			}
		}
		go s.rpcServer.ServeCodec(jsonrpc.NewServerCodec(conn))
	}
}

// Stop gracefully shuts down the daemon
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// Addr returns the bound listener address when the daemon is running.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}
