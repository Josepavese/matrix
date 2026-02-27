package daemon

import (
	"log/slog"
	"net/rpc"
	"net/rpc/jsonrpc"

	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/middleware"
)

// Server represents the JSON-RPC Daemon server
type Server struct {
	rpcServer *rpc.Server
	net       middleware.Network
	listener  middleware.ClosableListener
	vault     *vault.Vault
}

// NewServer initializes a new JSON-RPC Daemon
func NewServer(v *vault.Vault, n middleware.Network) *Server {
	return &Server{
		rpcServer: rpc.NewServer(),
		vault:     v,
		net:       n,
	}
}

// Start opens the TCP socket and begins serving JSON-RPC requests
func (s *Server) Start(addr string) error {
	// Register the Vault Service
	vaultSvc := NewVaultService(s.vault)
	if err := s.rpcServer.RegisterName("Vault", vaultSvc); err != nil {
		return err
	}

	l, err := s.net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = l

	slog.Info("matrix daemon started", "addr", addr, "protocol", "json-rpc")

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			slog.Error("accept error", "err", err)
			continue
		}
		go s.rpcServer.ServeCodec(jsonrpc.NewServerCodec(conn))
	}
}

// Stop gracefully shuts down the daemon
func (s *Server) Stop() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
