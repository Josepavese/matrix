package channelruntime

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Josepavese/matrix/internal/logic/channelcfg"
	"github.com/Josepavese/matrix/internal/logic/config"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/telegram"
)

// Factory creates messaging gateways from the neutral runtime registry.
type Factory interface {
	Name() string
	Build(reader middleware.ConfigReader, cfgMgr *config.Manager, router middleware.SessionRouter) (middleware.MessagingGateway, bool, error)
}

// StartAll starts every enabled messaging gateway registered in the runtime.
func StartAll(ctx context.Context, reader middleware.ConfigReader, cfgMgr *config.Manager, router middleware.SessionRouter, factories ...Factory) ([]middleware.MessagingGateway, error) {
	started := make([]middleware.MessagingGateway, 0, len(factories))
	for _, factory := range factories {
		gateway, enabled, err := factory.Build(reader, cfgMgr, router)
		if err != nil {
			return started, fmt.Errorf("%s gateway init failed: %w", factory.Name(), err)
		}
		if !enabled || gateway == nil {
			continue
		}
		if err := gateway.Start(ctx); err != nil {
			return started, fmt.Errorf("%s gateway start failed: %w", factory.Name(), err)
		}
		slog.Info("channel gateway started", "provider", factory.Name())
		started = append(started, gateway)
	}
	return started, nil
}

// StopAll stops all running gateways, returning the first error if any.
func StopAll(gateways []middleware.MessagingGateway) error {
	var firstErr error
	for _, gateway := range gateways {
		if err := gateway.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// DefaultFactories returns the built-in messaging gateway factories.
func DefaultFactories() []Factory {
	return []Factory{
		telegramFactory{},
	}
}

type telegramFactory struct{}

func (telegramFactory) Name() string { return "telegram" }

func (telegramFactory) Build(reader middleware.ConfigReader, cfgMgr *config.Manager, router middleware.SessionRouter) (middleware.MessagingGateway, bool, error) {
	cfg, _, err := channelcfg.LoadTelegramConfig(reader, cfgMgr)
	if err != nil {
		return nil, false, err
	}
	if !cfg.Enabled || cfg.Token == "" {
		return nil, false, nil
	}
	gateway, err := telegram.NewBot(cfg.Token, router)
	if err != nil {
		return nil, false, err
	}
	return gateway, true, nil
}
