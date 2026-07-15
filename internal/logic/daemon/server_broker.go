package daemon

import (
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
	"github.com/Josepavese/matrix/internal/middleware"
)

type runtimeBrokerConfig struct {
	store   middleware.Storage
	fs      middleware.FS
	home    string
	logFile string
	token   string
}

func listenerAddress(listener middleware.ClosableListener, fallback string) string {
	if listener.Addr() == nil {
		return fallback
	}
	return listener.Addr().String()
}

func (s *Server) prepareRuntimeBroker(addr string) error {
	if s.broker == nil {
		return nil
	}
	descriptor, err := runtimebroker.New(addr, s.broker.logFile)
	if err != nil {
		return err
	}
	s.broker.token = descriptor.Token
	return nil
}

func (s *Server) activateRuntimeBroker(addr string) (func(), error) {
	if s.broker == nil {
		return func() {}, nil
	}
	path := runtimebroker.Path(s.broker.home)
	descriptor, err := runtimebroker.New(addr, s.broker.logFile)
	if err != nil {
		return nil, err
	}
	if descriptor.Token != s.broker.token {
		descriptor.Token = s.broker.token
	}
	if err := runtimebroker.Write(s.broker.fs, path, descriptor); err != nil {
		return nil, fmt.Errorf("publish runtime broker: %w", err)
	}
	return func() { _ = runtimebroker.Remove(s.broker.fs, path) }, nil
}
