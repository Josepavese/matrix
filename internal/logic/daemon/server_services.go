package daemon

import "log/slog"

func (s *Server) registerServices(log *slog.Logger) error {
	if err := s.rpcServer.RegisterName("Vault", NewVaultService(s.vault, s.apiKey)); err != nil {
		return err
	}
	if s.broker != nil {
		if err := s.rpcServer.RegisterName("Storage", NewStorageService(s.broker.store, s.broker.token)); err != nil {
			return err
		}
	}
	if s.apiKey == "" {
		return nil
	}
	if err := s.rpcServer.RegisterName("Auth", &AuthService{apiKey: s.apiKey}); err != nil {
		return err
	}
	log.Info("daemon API key authentication enabled", "event", "daemon_auth_enabled")
	return nil
}
