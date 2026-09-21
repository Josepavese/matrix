// Package telegram implements a Telegram bot interface for the Matrix agent system.
package telegram

import (
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
	// chats correlates a remote ACP session with its chat, so elicitation
	// requests land in the conversation that started the run.
	chats *sessionChatIndex
	// elicitation renders registry events into chats; nil when disabled.
	elicitation   *elicitationUI
	unsubscribe   func()
	unsubscribeMu sync.Mutex
}

// thoughtMessenger implements ThoughtNotifier for Telegram.
// It creates a temporary "thinking" message and edits it in real-time.

type thoughtMessenger struct {
	api            *tgbotapi.BotAPI
	chatID         int64
	chats          *sessionChatIndex
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

func newThoughtMessenger(api *tgbotapi.BotAPI, chatID int64, replyToID int, chats *sessionChatIndex) *thoughtMessenger {
	return &thoughtMessenger{
		api:       api,
		chatID:    chatID,
		replyToID: replyToID,
		chats:     chats,
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
	// The turn tells us which remote session it runs on, which is exactly the
	// correlation key an elicitation carries.
	if t.chats != nil {
		t.chats.bind(agentSessionID, t.chatID)
	}
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
		chats:  newSessionChatIndex(),
	}, nil
}

// WithElicitationService enables the Telegram elicitation frontend. Without it
// the bot only routes messages, exactly as before.
