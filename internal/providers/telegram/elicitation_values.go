package telegram

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

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

func coerceOptionValue(field middleware.ElicitationField, raw string) (interface{}, error) {
	switch field.Type {
	case "boolean":
		switch raw {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return nil, fmt.Errorf("atteso un valore booleano")
		}
	case "number":
		return coerceElicitationValue(field, raw)
	default:
		return raw, nil
	}
}

func coerceElicitationValue(field middleware.ElicitationField, raw string) (interface{}, error) {
	switch field.Type {
	case "number":
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			// NaN and ±Inf parse but cannot be encoded back to JSON, which would
			// turn the answer into an internal protocol error.
			return nil, fmt.Errorf("atteso un numero finito")
		}
		return value, nil
	default:
		if raw == "" {
			return nil, fmt.Errorf("valore vuoto")
		}
		return raw, nil
	}
}

func fieldByName(request middleware.ElicitationRequest, name string) *middleware.ElicitationField {
	for index := range request.Fields {
		if request.Fields[index].Name == name {
			return &request.Fields[index]
		}
	}
	return nil
}

func fieldByIndex(request middleware.ElicitationRequest, index int) *middleware.ElicitationField {
	if index < 0 || index >= len(request.Fields) {
		return nil
	}
	return &request.Fields[index]
}

func cloneValues(values map[string]interface{}) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

// redactURLUserinfo removes credentials from a URL before it is displayed.
// Agents must not embed secrets in the URL; when one does, the chat must not
// echo the secret to everyone with access to the conversation.

type callbackPayload struct {
	token       string
	action      string
	fieldIndex  int
	optionIndex int
	optionValue string
}

func (p callbackPayload) encode() string {
	return strings.Join([]string{
		callbackDataPrefix, p.token, p.action, strconv.Itoa(p.fieldIndex), strconv.Itoa(p.optionIndex),
	}, ":")
}

func decodeCallbackData(data string) (callbackPayload, bool) {
	parts := strings.Split(data, ":")
	// A blank token can never identify a pending request, and treating
	// whitespace-only identifiers as valid contradicts how every other
	// identifier in this codebase is handled.
	if len(parts) != 5 || parts[0] != callbackDataPrefix || strings.TrimSpace(parts[1]) == "" {
		return callbackPayload{}, false
	}
	fieldIndex, err := strconv.Atoi(parts[3])
	if err != nil {
		return callbackPayload{}, false
	}
	optionIndex, err := strconv.Atoi(parts[4])
	if err != nil {
		return callbackPayload{}, false
	}
	return callbackPayload{token: parts[1], action: parts[2], fieldIndex: fieldIndex, optionIndex: optionIndex}, true
}

// ----------------------------------------------------------------------------
// Session correlation
// ----------------------------------------------------------------------------

// sessionChatIndex maps a remote ACP session to the chat that owns it.

type sessionChatIndex struct {
	mu    sync.RWMutex
	chats map[string]int64
}

func newSessionChatIndex() *sessionChatIndex {
	return &sessionChatIndex{chats: map[string]int64{}}
}

func (i *sessionChatIndex) bind(sessionID string, chatID int64) {
	sessionID = strings.TrimSpace(sessionID)
	if i == nil || sessionID == "" || chatID == 0 {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.chats[sessionID] = chatID
}

func (i *sessionChatIndex) lookup(sessionID string) (int64, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if i == nil || sessionID == "" {
		return 0, false
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	chatID, ok := i.chats[sessionID]
	return chatID, ok
}
