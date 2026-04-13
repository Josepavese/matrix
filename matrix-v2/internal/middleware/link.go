package middleware

import "context"

// MessagingGateway is the abstraction for a link provider (Telegram, WhatsApp, etc.).
// It bridges asynchronous messaging apps into the Matrix core by forwarding received messages
// into the SessionRouter and writing back the responses.
type MessagingGateway interface {
	// Start connects to the messaging platform (e.g. via long polling or starting a webhook server).
	Start(ctx context.Context) error

	// Stop disconnects the gateway and cleans up resources.
	Stop() error
}

// SessionRouter routes messages from an external channel to an agent via the SSOT Vault.
type SessionRouter interface {
	Route(ctx context.Context, channelID string, agentID string, input string, notifier ThoughtNotifier) (string, error)
}
