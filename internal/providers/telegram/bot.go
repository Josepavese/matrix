// Package telegram implements a Telegram bot interface for the Matrix agent system.
package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const telegramSafeMessageLimit = 3900

// Bot implements middleware.MessagingGateway for Telegram.
type Bot struct {
	token    string
	router   middleware.SessionRouter
	api      *tgbotapi.BotAPI
	stopCh   chan struct{}
	stopOnce sync.Once
}

// thoughtMessenger implements ThoughtNotifier for Telegram.
// It creates a temporary "thinking" message and edits it in real-time.
type thoughtMessenger struct {
	api            *tgbotapi.BotAPI
	chatID         int64
	replyToID      int
	messageID      int
	mu             sync.Mutex
	thoughts       string
	tools          string
	lastEdit       string
	rateLimit      time.Time
	agentID        string
	agentSessionID string
}

func newThoughtMessenger(api *tgbotapi.BotAPI, chatID int64, replyToID int) *thoughtMessenger {
	return &thoughtMessenger{
		api:       api,
		chatID:    chatID,
		replyToID: replyToID,
	}
}

// Send creates the initial "thinking" placeholder message as reply to the user.
func (t *thoughtMessenger) Send() error {
	msg := tgbotapi.NewMessage(t.chatID, "Matrix is routing...")
	msg.ReplyToMessageID = t.replyToID
	sent, err := t.api.Send(msg)
	if err != nil {
		return err
	}
	t.messageID = sent.MessageID
	return nil
}

// SetHeader stores agent/session metadata. Does NOT update the message —
// the edit will happen when the first real content arrives via OnThought.
func (t *thoughtMessenger) SetHeader(agentID, agentSessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.agentID = agentID
	t.agentSessionID = agentSessionID
}

// OnThought receives real-time thought/tool updates and edits the temporary message.
func (t *thoughtMessenger) OnThought(update middleware.ThoughtUpdate) {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch update.Type {
	case middleware.ThoughtTypeThinking:
		t.thoughts = update.Content // keep only latest thought
	case middleware.ThoughtTypeToolCall:
		if update.Content != "" {
			t.tools += "🔧 " + escapeHTML(truncateThought(update.Content, 80)) + "\n"
		} else {
			t.tools += "🔧 tool...\n"
		}
	case middleware.ThoughtTypeToolResult:
		// Brief result append
		if update.Content != "" {
			t.tools += "  ✅ " + escapeHTML(truncateThought(update.Content, 60)) + "\n"
		}
	}

	// Rate-limit Telegram edits to ~1 per second
	if time.Since(t.rateLimit) < time.Second {
		return
	}

	t.editMessage()
}

func (t *thoughtMessenger) editMessage() {
	if t.messageID == 0 {
		return
	}

	// Header: agent + session (short)
	var header string
	if t.agentID != "" {
		sid := t.agentSessionID
		if len(sid) > 8 {
			sid = sid[:8]
		}
		header = fmt.Sprintf("📎 <code>%s:%s</code>\n━━━━━━━━━━━━━━━━━━", escapeHTML(t.agentID), escapeHTML(sid))
	}

	var parts string
	if t.thoughts != "" {
		parts += "💭 <i>" + escapeHTML(truncateThought(t.thoughts, 300)) + "</i>"
	}
	if t.tools != "" {
		if parts != "" {
			parts += "\n\n"
		}
		parts += t.tools
	}

	// Don't edit if there's no real content yet — keep "💭 Sto pensando..." placeholder
	if parts == "" {
		return
	}

	text := header + "\n" + parts
	if text == t.lastEdit {
		return
	}
	t.lastEdit = text
	t.rateLimit = time.Now()

	edit := tgbotapi.NewEditMessageText(t.chatID, t.messageID, text)
	edit.ParseMode = tgbotapi.ModeHTML
	if _, err := t.api.Send(edit); err != nil {
		slog.Warn("thought message edit failed", "error", err, "text_len", len(text))
	}
}

// Delete removes the temporary thinking message.
func (t *thoughtMessenger) Delete() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.messageID == 0 {
		return
	}
	deleteMsg := tgbotapi.NewDeleteMessage(t.chatID, t.messageID)
	if _, err := t.api.Request(deleteMsg); err != nil {
		slog.Warn("thought message delete failed", "error", err)
	}
	t.messageID = 0
}

// FormattedHeader returns the platform-styled agent/session label for the final response.
func (t *thoughtMessenger) FormattedHeader() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.agentID == "" {
		return ""
	}
	sid := t.agentSessionID
	if len(sid) > 8 {
		sid = sid[:8]
	}
	return fmt.Sprintf("📎 %s:%s\n━━━━━━━━━━━━━━━━━━", t.agentID, sid)
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func truncateThought(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// NewBot initializes a new Telegram linkage using long-polling.
func NewBot(token string, router middleware.SessionRouter) (*Bot, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram token cannot be empty")
	}

	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telegram bot: %w", err)
	}

	return &Bot{
		token:  token,
		router: router,
		api:    api,
		stopCh: make(chan struct{}),
	}, nil
}

// Start begins the long-polling event loop. Let it run in a goroutine.
func (b *Bot) Start(ctx context.Context) error {
	log := slog.With("component", "telegram_gateway")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	log.Info("starting telegram gateway", "event", "gateway_starting", "bot", b.api.Self.UserName)
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
	if update.Message == nil {
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
	thought := newThoughtMessenger(b.api, update.Message.Chat.ID, update.Message.MessageID)
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
	b.api.StopReceivingUpdates()
	b.stopOnce.Do(func() { close(b.stopCh) })
	return nil
}
