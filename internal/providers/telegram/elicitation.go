package telegram

import (
	"context"
	"log/slog"
	"strconv"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
)

// ----------------------------------------------------------------------------
// Telegram elicitation frontend
// ----------------------------------------------------------------------------
//
// This is a channel frontend over the neutral elicitation registry: it observes
// registry events, renders the question in the originating chat, and answers
// through the same Service the HTTP surface uses. The registry stays the single
// source of truth; Telegram owns only presentation and per-chat correlation.

// telegramSender is the slice of the Telegram API this frontend needs, so the
// rendering and state machine stay testable without network access.

type telegramSender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

// callbackDataPrefix marks callback payloads owned by elicitation.

const callbackDataPrefix = "el"

// telegramMaxKeyboardButtons bounds the whole inline keyboard. Telegram
// rejects oversized keyboards, so a pathological form must degrade to a
// truncated, still-answerable prompt instead of an outright API error.

const telegramMaxKeyboardButtons = 100

// keyboardControlButtons is the space reserved for the always-present actions
// (confirm, decline, cancel).

const keyboardControlButtons = 2

type pendingElicitation struct {
	token     string
	request   middleware.ElicitationRequest
	chatID    int64
	messageID int
	answers   map[string]interface{}
	// awaitingField is set when the request needs a free-text reply. The next
	// message in the chat is consumed as that answer instead of being routed.
	awaitingField string
}

// elicitationUI renders elicitation requests into a Telegram chat.

type elicitationUI struct {
	api     telegramSender
	service *elicitation.Service
	// chats maps a remote ACP session to the chat that owns it. The bot fills
	// it from ThoughtNotifier.SetHeader, which already receives the session id
	// at every turn start, so no new correlation channel is needed.
	chats *sessionChatIndex

	mu      sync.Mutex
	pending map[string]*pendingElicitation // by token
	byChat  map[int64]string               // chat awaiting a text answer
	nextID  int
}

func newElicitationUI(api telegramSender, service *elicitation.Service, chats *sessionChatIndex) *elicitationUI {
	return &elicitationUI{
		api:     api,
		service: service,
		chats:   chats,
		pending: map[string]*pendingElicitation{},
		byChat:  map[int64]string{},
	}
}

// subscribe wires the registry events into this chat surface.

func (ui *elicitationUI) subscribe() func() {
	if ui == nil || ui.service == nil {
		return func() {}
	}
	return ui.service.Subscribe(func(event elicitation.Event) {
		switch event.Kind {
		case elicitation.EventOpened:
			ui.onOpened(context.Background(), event.Request)
		case elicitation.EventResolved:
			ui.onResolved(event.Request.ID, event.Outcome)
		}
	})
}

func (ui *elicitationUI) onOpened(_ context.Context, request middleware.ElicitationRequest) {
	chatID, ok := ui.chats.lookup(request.SessionID)
	if !ok {
		slog.Info("elicitation has no telegram chat", "event", "elicitation_no_chat",
			"elicitation_id", request.ID, "agent_id", request.AgentID, "session_id", request.SessionID)
		return
	}
	pending := ui.register(request, chatID)
	message := tgbotapi.NewMessage(chatID, renderElicitation(pending))
	message.ReplyMarkup = elicitationKeyboard(pending)
	sent, err := ui.api.Send(message)
	if err != nil {
		slog.Warn("failed to send elicitation prompt", "event", "elicitation_send_failed",
			"elicitation_id", request.ID, "chat_id", chatID, "error", err)
		ui.forget(pending.token)
		return
	}
	ui.setMessageID(pending.token, sent.MessageID)
	slog.Info("elicitation prompt sent", "event", "elicitation_prompt_sent",
		"elicitation_id", request.ID, "chat_id", chatID, "message_id", sent.MessageID, "agent_id", request.AgentID)
}

func (ui *elicitationUI) onResolved(id string, outcome middleware.ElicitationOutcome) {
	pending := ui.takeByElicitationID(id)
	if pending == nil {
		return
	}
	ui.edit(pending, renderResolution(pending, outcome), nil)
}

// consumeText answers a pending free-text elicitation from a chat message.
// It reports true when the message was consumed and must not be routed.

func (ui *elicitationUI) register(request middleware.ElicitationRequest, chatID int64) *pendingElicitation {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	ui.nextID++
	token := "e" + strconv.Itoa(ui.nextID)
	pending := &pendingElicitation{
		token:   token,
		request: request,
		chatID:  chatID,
		answers: map[string]interface{}{},
	}
	ui.pending[token] = pending
	if field := freeTextField(request); field != nil {
		pending.awaitingField = field.Name
		ui.byChat[chatID] = token
	}
	return pending
}

func (ui *elicitationUI) setMessageID(token string, messageID int) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if pending := ui.pending[token]; pending != nil {
		pending.messageID = messageID
	}
}

func (ui *elicitationUI) takeByElicitationID(id string) *pendingElicitation {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	for token, pending := range ui.pending {
		if pending.request.ID == id {
			delete(ui.pending, token)
			delete(ui.byChat, pending.chatID)
			return pending
		}
	}
	return nil
}

func (ui *elicitationUI) takeToken(token string) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if pending := ui.pending[token]; pending != nil {
		delete(ui.pending, token)
		delete(ui.byChat, pending.chatID)
	}
}

func (ui *elicitationUI) forget(token string) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if pending := ui.pending[token]; pending != nil {
		delete(ui.pending, token)
		delete(ui.byChat, pending.chatID)
	}
}

func (ui *elicitationUI) edit(pending *pendingElicitation, text string, keyboard *tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(pending.chatID, pending.messageID, text)
	if keyboard != nil {
		edit.ReplyMarkup = keyboard
	} else {
		empty := tgbotapi.NewInlineKeyboardMarkup()
		edit.ReplyMarkup = &empty
	}
	if _, err := ui.api.Request(edit); err != nil {
		slog.Debug("failed to edit elicitation message", "event", "elicitation_edit_failed",
			"elicitation_id", pending.request.ID, "error", err)
	}
}

func (ui *elicitationUI) replyInvalid(pending *pendingElicitation, err error) {
	if _, sendErr := ui.api.Send(tgbotapi.NewMessage(pending.chatID, "Valore non valido: "+err.Error())); sendErr != nil {
		slog.Debug("failed to report invalid elicitation value", "error", sendErr)
	}
}

func (ui *elicitationUI) ackCallback(callbackID, text string) {
	if callbackID == "" {
		return
	}
	if _, err := ui.api.Request(tgbotapi.NewCallback(callbackID, text)); err != nil {
		slog.Debug("failed to answer callback", "error", err)
	}
}

// ----------------------------------------------------------------------------
// Rendering
// ----------------------------------------------------------------------------
