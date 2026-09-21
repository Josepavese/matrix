// Package telegram implements a Telegram bot interface for the Matrix agent system.
package telegram

import (
	"github.com/Josepavese/matrix/internal/logic/elicitation"
)

func (b *Bot) WithElicitationService(service *elicitation.Service) *Bot {
	if service == nil {
		return b
	}
	b.elicitation = newElicitationUI(b.api, service, b.chats)
	return b
}

// subscribeElicitation starts forwarding registry events into chats.

func (b *Bot) subscribeElicitation() {
	if b.elicitation == nil {
		return
	}
	b.unsubscribeMu.Lock()
	if b.unsubscribe == nil {
		b.unsubscribe = b.elicitation.subscribe()
	}
	b.unsubscribeMu.Unlock()
}

func (b *Bot) stopElicitation() {
	b.unsubscribeMu.Lock()
	unsubscribe := b.unsubscribe
	b.unsubscribe = nil
	b.unsubscribeMu.Unlock()
	if unsubscribe != nil {
		unsubscribe()
	}
}

// Start begins the long-polling event loop. Let it run in a goroutine.
