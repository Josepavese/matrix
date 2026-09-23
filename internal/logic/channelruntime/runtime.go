package channelruntime

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Josepavese/matrix/internal/logic/channelcfg"
	"github.com/Josepavese/matrix/internal/logic/config"
	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/telegram"
)

// Deps carries the neutral runtime services a gateway may need beyond routing.
// Elicitation is nil when the surface is disabled.
type Deps struct {
	Elicitation *elicitation.Service
}

// Factory creates messaging gateways from the neutral runtime registry.
type Factory interface {
	Name() string
	Build(reader middleware.ConfigReader, cfgMgr *config.Manager, router middleware.SessionRouter, deps Deps) (middleware.MessagingGateway, bool, error)
}

// StartAll starts every enabled messaging gateway registered in the runtime.
func StartAll(ctx context.Context, reader middleware.ConfigReader, cfgMgr *config.Manager, router middleware.SessionRouter, deps Deps, factories ...Factory) ([]middleware.MessagingGateway, error) {
	started := make([]middleware.MessagingGateway, 0, len(factories))
	for _, factory := range factories {
		gateway, enabled, err := factory.Build(reader, cfgMgr, router, deps)
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

func (telegramFactory) Build(reader middleware.ConfigReader, cfgMgr *config.Manager, router middleware.SessionRouter, deps Deps) (middleware.MessagingGateway, bool, error) {
	cfg, _, err := channelcfg.LoadTelegramConfig(reader, cfgMgr)
	if err != nil {
		return nil, false, err
	}
	if !cfg.Enabled || cfg.Token == "" {
		return nil, false, nil
	}
	// Fail closed: a startable channel with an empty admin list would answer
	// anyone who can reach the bot, so it is refused loudly instead.
	if len(cfg.Admins) == 0 {
		return nil, false, fmt.Errorf("telegram channel is enabled but channel.telegram.admins is empty: refusing to start (fail closed); set at least one Telegram user id")
	}
	gateway, err := telegram.NewBot(cfg.Token, router, cfg.Admins)
	if err != nil {
		return nil, false, err
	}
	if deps.Elicitation != nil {
		gateway.WithElicitationService(deps.Elicitation)
	}
	return gateway, true, nil
}
