package telegram

import (
	"fmt"
	"net/url"
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

func renderElicitation(pending *pendingElicitation) string {
	request := pending.request
	var b strings.Builder
	b.WriteString("❓ Richiesta di input\n")
	if request.AgentID != "" {
		b.WriteString("Agente: " + request.AgentID + "\n")
	}
	if request.Message != "" {
		b.WriteString("\n" + request.Message + "\n")
	}
	if request.Mode == middleware.ElicitationModeURL {
		b.WriteString("\nDestinazione: " + urlHost(request.URL) + "\n")
		b.WriteString(redactURLUserinfo(request.URL) + "\n")
		b.WriteString("\nMatrix non apre il link: aprilo tu e conferma il consenso.\n")
		return b.String()
	}
	b.WriteString("\n")
	renderFormFields(&b, pending)
	renderFormFooter(&b, pending)
	return b.String()
}

// renderFormFields lists one line per field with its current state.
func renderFormFields(b *strings.Builder, pending *pendingElicitation) {
	for _, field := range pending.request.Fields {
		marker := "▫️"
		value, answered := pending.answers[field.Name]
		if answered {
			marker = "✅"
		}
		line := marker + " " + fieldLabel(field)
		switch {
		case answered:
			line += ": " + fmt.Sprintf("%v", value)
		case field.Required:
			line += " (obbligatorio)"
		}
		b.WriteString(line + "\n")
		if detail := fieldDetail(field); detail != "" && !answered {
			b.WriteString("    " + detail + "\n")
		}
	}
}

// renderFormFooter explains what is still missing and what the next tap does.
func renderFormFooter(b *strings.Builder, pending *pendingElicitation) {
	if optionsTruncated(pending) {
		b.WriteString("\n⚠️ Alcune opzioni non sono mostrate (limite della chat): usa l'API HTTP per rispondere per intero.\n")
	}
	if missing := unansweredRequired(pending); len(missing) > 0 {
		b.WriteString("\nCompleta i campi e premi Invia.\n")
		return
	}
	b.WriteString("\nPremi Invia per confermare.\n")
}

func renderResolution(pending *pendingElicitation, outcome middleware.ElicitationOutcome) string {
	base := renderElicitation(pending)
	switch outcome.Action {
	case middleware.ElicitationActionAccept:
		return base + "\n✅ Risposta inviata."
	case middleware.ElicitationActionDecline:
		return base + "\n🚫 Richiesta rifiutata."
	default:
		return base + "\n⌛ Richiesta annullata o scaduta."
	}
}

func elicitationKeyboard(pending *pendingElicitation) *tgbotapi.InlineKeyboardMarkup {
	request := pending.request
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(request.Fields)+2)
	base := tgbotapi.NewInlineKeyboardMarkup()
	if request.Mode == middleware.ElicitationModeURL {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(button("✅ Consento ad aprirlo", callbackPayload{token: pending.token, action: "url-accept"})))
	} else {
		budget := telegramMaxKeyboardButtons - keyboardControlButtons
		if len(unansweredRequired(pending)) == 0 {
			budget-- // reserve the Invia row
		}
		for index, field := range request.Fields {
			if _, answered := pending.answers[field.Name]; answered {
				continue
			}
			options := fieldOptions(field)
			for optionIndex, option := range options {
				if budget <= 0 {
					break
				}
				budget--
				label := option.Label
				if label == "" {
					label = option.Value
				}
				rows = append(rows, tgbotapi.NewInlineKeyboardRow(button(label, callbackPayload{
					token: pending.token, action: "pick", fieldIndex: index, optionIndex: optionIndex,
				})))
			}
		}
		if len(unansweredRequired(pending)) == 0 {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				button("📨 Invia", callbackPayload{token: pending.token, action: "confirm"}),
			))
		}
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		button("🚫 Rifiuta", callbackPayload{token: pending.token, action: "decline"}),
		button("✖️ Annulla", callbackPayload{token: pending.token, action: "cancel"}),
	))
	base.InlineKeyboard = rows
	return &base
}

func button(label string, payload callbackPayload) tgbotapi.InlineKeyboardButton {
	return tgbotapi.NewInlineKeyboardButtonData(label, payload.encode())
}

func fieldLabel(field middleware.ElicitationField) string {
	if strings.TrimSpace(field.Title) != "" {
		return field.Title
	}
	if strings.TrimSpace(field.Description) != "" {
		return field.Description
	}
	return field.Name
}

func fieldDetail(field middleware.ElicitationField) string {
	if field.Type == "string" || field.Type == "number" {
		return "Rispondi con un messaggio."
	}
	return ""
}

// fieldOptions returns the selectable options, synthesising Yes/No for boolean
// fields so every supported field type is answerable with one tap.

func fieldOptions(field middleware.ElicitationField) []middleware.ElicitationOption {
	if len(field.Options) > 0 {
		return field.Options
	}
	if field.Type == "boolean" {
		return []middleware.ElicitationOption{{Value: "true", Label: "Sì"}, {Value: "false", Label: "No"}}
	}
	return nil
}

// freeTextField reports the single free-text field a chat reply can answer.
// Requests mixing free text with other fields are answered from the HTTP
// surface instead of guessing at a multi-field text parse.

func freeTextField(request middleware.ElicitationRequest) *middleware.ElicitationField {
	if request.Mode != middleware.ElicitationModeForm || len(request.Fields) != 1 {
		return nil
	}
	field := request.Fields[0]
	if field.Type == "string" || field.Type == "number" {
		return &field
	}
	return nil
}

// optionsTruncated reports whether the rendered keyboard had to drop options
// to stay inside Telegram's limits.

func optionsTruncated(pending *pendingElicitation) bool {
	budget := telegramMaxKeyboardButtons - keyboardControlButtons
	if len(unansweredRequired(pending)) == 0 {
		budget--
	}
	for _, field := range pending.request.Fields {
		if _, answered := pending.answers[field.Name]; answered {
			continue
		}
		budget -= len(fieldOptions(field))
	}
	return budget < 0
}

func unansweredRequired(pending *pendingElicitation) []string {
	var missing []string
	for _, field := range pending.request.Fields {
		if !field.Required {
			continue
		}
		if _, ok := pending.answers[field.Name]; !ok {
			missing = append(missing, field.Name)
		}
	}
	return missing
}

// coerceOptionValue turns a button's string value into the JSON type the
// schema declares. Boolean choices are rendered as text buttons, so without
// this the stored answer is the string "true" and validation — correctly —
// refuses a boolean field answered from chat.

func redactURLUserinfo(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User == nil {
		return raw
	}
	user := parsed.User.Username()
	parsed.User = url.UserPassword(user, "redacted")
	return parsed.String()
}

func urlHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "(host non riconosciuto)"
	}
	return parsed.Host
}

// ----------------------------------------------------------------------------
// Callback payload encoding
// ----------------------------------------------------------------------------

// callbackPayload is the compact, token-based callback data. Telegram caps
// callback data at 64 bytes, so the registry id (which can be long) never
// travels in the button; only the short UI token does.
