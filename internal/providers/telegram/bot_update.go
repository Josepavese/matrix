// Package telegram implements a Telegram bot interface for the Matrix agent system.
package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) Start(ctx context.Context) error {
	log := slog.With("component", "telegram_gateway")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	u.AllowedUpdates = []string{"message", "callback_query"}

	b.subscribeElicitation()
	defer b.stopElicitation()

	log.Info("starting telegram gateway", "event", "gateway_starting", "bot", b.api.Self.UserName,
		"elicitation", b.elicitation != nil)
	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			log.Info("shutting down telegram gateway", "event", "gateway_stopped", "reason", "context_cancelled")
			return nil
		case <-b.stopCh:
			log.Info("shutting down telegram gateway", "event", "gateway_stopped", "reason", "stop_requested")
			return nil
		case update, ok := <-updates:
			if !ok {
				log.Warn("telegram updates channel closed", "event", "updates_closed")
				return nil
			}
			b.handleUpdate(ctx, log, update)
		}
	}
}

func (b *Bot) handleUpdate(ctx context.Context, log *slog.Logger, update tgbotapi.Update) {
	// Authorisation runs before any dispatch, so a rejected update can reach
	// neither an agent nor a system tool, a pending elicitation or a button.
	if !b.admins.Authorize(log, update) {
		return
	}
	if update.CallbackQuery != nil {
		if b.elicitation != nil && b.elicitation.handleCallback(update.CallbackQuery) {
			log.Info("elicitation callback handled", "event", "elicitation_callback",
				"chat_id", callbackChatID(update.CallbackQuery), "data_len", len(update.CallbackQuery.Data))
		}
		return
	}
	if update.Message == nil {
		return
	}

	// A pending elicitation owns the next message in its chat: it is an answer,
	// not a new prompt.
	if b.elicitation != nil && update.Message.Text != "" && b.elicitation.consumeText(update.Message.Chat.ID, update.Message.Text) {
		log.Info("elicitation answer consumed from chat", "event", "elicitation_text_consumed",
			"chat_id", update.Message.Chat.ID, "text_len", len(update.Message.Text))
		return
	}

	text := update.Message.Text
	channelID := fmt.Sprintf("telegram_%d", update.Message.Chat.ID)
	log.Info("received telegram message", "event", "message_received", "channel", channelID, "chat_id", update.Message.Chat.ID, "user_id", update.Message.From.ID, "message_id", update.Message.MessageID, "text_len", len(text))

	go b.processMessage(ctx, log, update, channelID)
}

func (b *Bot) processMessage(ctx context.Context, log *slog.Logger, update tgbotapi.Update, channelID string) {
	if _, err := b.api.Request(tgbotapi.NewChatAction(update.Message.Chat.ID, tgbotapi.ChatTyping)); err != nil {
		log.Warn("failed to send typing action", "event", "typing_action_failed", "channel", channelID, "chat_id", update.Message.Chat.ID, "error", err)
	}

	// Create a live "thinking" message that gets updated in real-time
	var notifier middleware.ThoughtNotifier
	thought := newThoughtMessenger(b.api, update.Message.Chat.ID, update.Message.MessageID, b.chats)
	if err := thought.Send(); err != nil {
		log.Warn("failed to send thinking placeholder", "event", "thinking_placeholder_failed", "error", err)
	} else {
		notifier = thought
	}

	response, err := b.router.Route(ctx, channelID, "", update.Message.Text, notifier)

	// Remove the thinking placeholder before sending the final response
	thought.Delete()

	if err != nil {
		log.Error("failed to route telegram message", "event", "route_failed", "channel", channelID, "chat_id", update.Message.Chat.ID, "message_id", update.Message.MessageID, "error", err)
		response = fmt.Sprintf("Matrix System Error: %v", err)
	}
	if response == "" {
		log.Warn("empty response from route", "event", "empty_response", "channel", channelID)
		return
	}

	// Prepend platform-styled agent/session label above the response
	if header := thought.FormattedHeader(); header != "" {
		response = header + "\n" + response
	}

	log.Info("sending telegram response", "event", "response_sending", "channel", channelID, "chat_id", update.Message.Chat.ID, "reply_to", update.Message.MessageID, "response_len", len(response))

	if err := b.sendResponse(update.Message.Chat.ID, update.Message.MessageID, response); err != nil {
		log.Error("failed to reply to telegram message", "event", "response_failed", "channel", channelID, "error", err, "chat_id", update.Message.Chat.ID)
		return
	}
	log.Info("telegram response sent successfully", "event", "response_sent", "channel", channelID, "chat_id", update.Message.Chat.ID)
}

func (b *Bot) sendResponse(chatID int64, replyTo int, response string) error {
	chunks := splitTelegramMessage(response, telegramSafeMessageLimit)
	for i, chunk := range chunks {
		msg := tgbotapi.NewMessage(chatID, chunk)
		if i == 0 {
			msg.ReplyToMessageID = replyTo
		}
		if _, err := b.api.Send(msg); err != nil {
			return err
		}
	}
	return nil
}

func splitTelegramMessage(text string, limit int) []string {
	if limit <= 0 {
		limit = telegramSafeMessageLimit
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return []string{text}
	}
	chunks := make([]string, 0, len(runes)/limit+1)
	for len(runes) > limit {
		splitAt := limit
		for i := limit - 1; i > limit/2; i-- {
			if runes[i] == '\n' {
				splitAt = i + 1
				break
			}
		}
		chunk := strings.TrimSpace(string(runes[:splitAt]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		runes = runes[splitAt:]
	}
	if tail := strings.TrimSpace(string(runes)); tail != "" {
		chunks = append(chunks, tail)
	}
	return chunks
}

// Stop cleanly terminates the gateway. Safe to call multiple times.

func (b *Bot) Stop() error {
	b.stopElicitation()
	b.api.StopReceivingUpdates()
	b.stopOnce.Do(func() { close(b.stopCh) })
	return nil
}

func callbackChatID(cb *tgbotapi.CallbackQuery) int64 {
	if cb == nil || cb.Message == nil || cb.Message.Chat == nil {
		return 0
	}
	return cb.Message.Chat.ID
}
