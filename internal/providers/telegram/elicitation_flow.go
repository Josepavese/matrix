package telegram

import (
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

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

func (ui *elicitationUI) consumeText(chatID int64, text string) bool {
	ui.mu.Lock()
	token, ok := ui.byChat[chatID]
	if !ok {
		ui.mu.Unlock()
		return false
	}
	pending := ui.pending[token]
	if pending == nil || pending.awaitingField == "" {
		delete(ui.byChat, chatID)
		ui.mu.Unlock()
		return false
	}
	value := strings.TrimSpace(text)
	field := fieldByName(pending.request, pending.awaitingField)
	if field == nil {
		delete(ui.byChat, chatID)
		ui.mu.Unlock()
		return false
	}
	typed, err := coerceElicitationValue(*field, value)
	if err != nil {
		ui.mu.Unlock()
		ui.replyInvalid(pending, err)
		return true
	}
	pending.answers[field.Name] = typed
	pending.awaitingField = ""
	delete(ui.byChat, chatID)
	keyboard := elicitationKeyboard(pending)
	rendered := renderElicitation(pending)
	ui.mu.Unlock()
	ui.edit(pending, rendered, keyboard)
	return true
}

// handleCallback answers an inline-keyboard interaction. It reports true when
// the callback belonged to an elicitation.

func (ui *elicitationUI) handleCallback(cb *tgbotapi.CallbackQuery) bool {
	parsed, ok := decodeCallbackData(cb.Data)
	if !ok {
		return false
	}
	ui.mu.Lock()
	pending := ui.pending[parsed.token]
	if pending == nil {
		ui.mu.Unlock()
		ui.ackCallback(cb.ID, "Richiesta già risolta.")
		return true
	}
	if cb.Message != nil && cb.Message.Chat != nil && cb.Message.Chat.ID != pending.chatID {
		ui.mu.Unlock()
		ui.ackCallback(cb.ID, "Questa richiesta appartiene a un'altra chat.")
		return true
	}
	field := fieldByIndex(pending.request, parsed.fieldIndex)
	if field != nil {
		if options := fieldOptions(*field); parsed.optionIndex >= 0 && parsed.optionIndex < len(options) {
			parsed.optionValue = options[parsed.optionIndex].Value
		}
	}
	ack, resolved := ui.applyCallbackLocked(pending, field, parsed)
	keyboard := elicitationKeyboard(pending)
	rendered := renderElicitation(pending)
	ui.mu.Unlock()

	if resolved != nil {
		ui.takeToken(pending.token)
		if ui.service.Respond(pending.request.ID, *resolved) {
			ui.edit(pending, renderResolution(pending, *resolved), nil)
			ui.ackCallback(cb.ID, "Risposta inviata.")
			return true
		}
		ui.edit(pending, "⌛ Questa richiesta è già scaduta.", nil)
		ui.ackCallback(cb.ID, "Richiesta scaduta.")
		return true
	}
	ui.ackCallback(cb.ID, ack)
	ui.edit(pending, rendered, keyboard)
	return true
}

// applyCallbackLocked folds one button press into the pending answers. It
// returns a terminal outcome when the user confirmed, declined or cancelled,
// otherwise a short acknowledgement string.

func (ui *elicitationUI) applyCallbackLocked(pending *pendingElicitation, field *middleware.ElicitationField, parsed callbackPayload) (ack string, resolved *middleware.ElicitationOutcome) {
	switch parsed.action {
	case "confirm":
		return applyConfirmAnswer(pending)
	case "decline":
		outcome := middleware.DeclineElicitation()
		return "", &outcome
	case "cancel":
		outcome := middleware.CancelElicitation()
		return "", &outcome
	case "url-accept":
		outcome := middleware.AcceptElicitation(nil)
		return "", &outcome
	case "pick":
		return applyOptionPick(pending, field, parsed)
	default:
		// An unknown or empty action used to fall through to the option branch
		// and silently select option 0, which would turn any future action name
		// (or a stale keyboard) into a wrong answer.
		return "Azione non riconosciuta.", nil
	}
}

// applyConfirmAnswer builds the accepted outcome the same way the HTTP surface
// does: through the shared validator, so a crafted or stale callback cannot send
// the agent an answer the schema forbids.
func applyConfirmAnswer(pending *pendingElicitation) (string, *middleware.ElicitationOutcome) {
	if missing := unansweredRequired(pending); len(missing) > 0 {
		return "Completa i campi obbligatori: " + strings.Join(missing, ", "), nil
	}
	values := cloneValues(pending.answers)
	if err := middleware.ValidateElicitationValues(pending.request, values); err != nil {
		return "Risposta non valida: " + err.Error(), nil
	}
	outcome := middleware.AcceptElicitation(values)
	return "", &outcome
}

// applyOptionPick records one constrained choice, coercing it to the type the
// schema declares.
func applyOptionPick(pending *pendingElicitation, field *middleware.ElicitationField, parsed callbackPayload) (string, *middleware.ElicitationOutcome) {
	if field == nil || parsed.optionValue == "" {
		return "Selezione non valida.", nil
	}
	value, err := coerceOptionValue(*field, parsed.optionValue)
	if err != nil {
		return "Selezione non valida.", nil
	}
	pending.answers[field.Name] = value
	return fieldLabel(*field) + ": " + optionDisplayLabel(value, parsed.optionValue), nil
}

// optionDisplayLabel renders a stored answer for the acknowledgement line.
func optionDisplayLabel(value interface{}, fallback string) string {
	if option, ok := value.(bool); ok {
		return map[bool]string{true: "Sì", false: "No"}[option]
	}
	return fallback
}
