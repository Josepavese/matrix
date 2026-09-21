package telegram

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
)

// ----------------------------------------------------------------------------
// Adversarial Telegram frontend suite
// ----------------------------------------------------------------------------

func askAsync(t *testing.T, service *elicitation.Service, request middleware.ElicitationRequest) chan middleware.ElicitationOutcome {
	t.Helper()
	outcomes := make(chan middleware.ElicitationOutcome, 1)
	go func() { outcomes <- service.Ask(context.Background(), request) }()
	return outcomes
}

// TestTelegramConcurrentTapsHaveOneWinner covers the impatient user: mashing a
// button must not deliver two answers, and the loser must not claim success.
func TestTelegramConcurrentTapsHaveOneWinner(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	outcomes := askAsync(t, service, choiceRequest())
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}
	rows := keyboardRows(t, api.sentMessages()[0])
	confirmLabel := ""

	// Select once, then hammer Invia.
	if !ui.handleCallback(callbackFor(t, rows, "Allow once", 0)) {
		t.Fatal("selection not handled")
	}
	confirmLabel = "📨 Invia"
	keyboard := api.lastKeyboard().InlineKeyboard

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ui.handleCallback(callbackFor(t, keyboard, confirmLabel, 0))
		}()
	}
	wg.Wait()

	select {
	case outcome := <-outcomes:
		if outcome.Action != middleware.ElicitationActionAccept {
			t.Fatalf("agent must receive the accepted answer, got %q", outcome.Action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent taps never resolved the request")
	}
	if len(service.Pending()) != 0 {
		t.Fatal("resolved request must leave the registry")
	}
}

// TestTelegramLateCallbackIsRejected keeps a stale keyboard harmless after the
// request was answered elsewhere.
func TestTelegramLateCallbackIsRejected(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	askAsync(t, service, choiceRequest())
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}
	if !service.Respond("session:sess_1", middleware.DeclineElicitation()) {
		t.Fatal("external answer refused")
	}
	if !api.waitForEdited(time.Second) {
		t.Fatal("prompt was not retracted")
	}
	// The button still exists in the chat; tapping it must be acknowledged and
	// must not resurrect or double-answer the request.
	late := &tgbotapi.CallbackQuery{
		ID:      "late",
		Data:    "el:e1:confirm:0:0",
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 4242}},
	}
	if !ui.handleCallback(late) {
		t.Fatal("late callback must be handled")
	}
	if len(api.acked) == 0 {
		t.Fatal("late callback must be acknowledged to the client")
	}
	if len(service.Pending()) != 0 {
		t.Fatal("a late callback must not recreate the request")
	}
}

// TestTelegramCallbackFromAnotherChatIsRefused makes cross-chat tampering
// impossible even if somebody replays callback data.
func TestTelegramCallbackFromAnotherChatIsRefused(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	askAsync(t, service, choiceRequest())
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}
	foreign := &tgbotapi.CallbackQuery{
		ID:      "foreign",
		Data:    "el:e1:confirm:0:0",
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 9999}},
	}
	if !ui.handleCallback(foreign) {
		t.Fatal("foreign callback must be handled (and refused)")
	}
	if len(service.Pending()) != 1 {
		t.Fatal("a callback from another chat must not resolve the request")
	}
}

// TestTelegramMalformedCallbackDataIsIgnored keeps garbage payloads from
// crashing or hijacking the surface.
func TestTelegramMalformedCallbackDataIsIgnored(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, _, _ := newTestUI(t, service)
	for _, data := range []string{
		"", "x", "el", "el:e1", "el:e1:pick", "el:e1:pick:0", "el:e1:pick:0:0:0",
		"el:e1:pick:a:b", "other:e1:pick:0:0", "el::pick:0:0",
	} {
		if ui.handleCallback(&tgbotapi.CallbackQuery{ID: "cb", Data: data}) {
			t.Fatalf("payload %q must not be treated as an elicitation callback", data)
		}
	}
}

// TestTelegramOutOfRangeOptionIsIgnored proves an index that no longer matches
// the schema cannot fabricate an answer.
func TestTelegramOutOfRangeOptionIsIgnored(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	askAsync(t, service, choiceRequest())
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}
	for _, data := range []string{"el:e1:pick:9:0", "el:e1:pick:0:9", "el:e1:pick:-1:-1"} {
		if !ui.handleCallback(&tgbotapi.CallbackQuery{ID: "cb", Data: data, Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 4242}}}) {
			t.Fatalf("payload %q must be handled", data)
		}
	}
	if len(service.Pending()) != 1 {
		t.Fatal("an out-of-range option must not resolve the request")
	}
	// Confirming without a valid selection must not accept either.
	ui.handleCallback(&tgbotapi.CallbackQuery{ID: "cb2", Data: "el:e1:confirm:0:0", Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 4242}}})
	if len(service.Pending()) != 1 {
		t.Fatal("confirm without an answer must not resolve the request")
	}
}

// TestTelegramMultiFieldChoiceFlow covers forms with several constrained
// fields: every required field must be answered before Invia appears.
func TestTelegramMultiFieldChoiceFlow(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	request := middleware.ElicitationRequest{
		ID: "session:sess_1", AgentID: "codex", Mode: middleware.ElicitationModeForm, SessionID: "sess_1",
		Message: "Configure the run",
		Fields: []middleware.ElicitationField{
			{Name: "db", Title: "Database", Type: "enum", Required: true,
				Options: []middleware.ElicitationOption{{Value: "postgres", Label: "Postgres"}, {Value: "sqlite", Label: "SQLite"}}},
			{Name: "cache", Title: "Cache", Type: "boolean", Required: true},
			{Name: "note", Title: "Note", Type: "string"},
		},
	}
	outcomes := askAsync(t, service, request)
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}
	rows := keyboardRows(t, api.sentMessages()[0])
	labels := buttonLabels(rows)
	if !containsLabel(labels, "Postgres") || !containsLabel(labels, "Sì") {
		t.Fatalf("every constrained field must be offered: %v", labels)
	}
	if contains(labels, "📨 Invia") {
		t.Fatalf("Invia must wait for all required fields: %v", labels)
	}

	if !ui.handleCallback(callbackFor(t, rows, "Postgres", 0)) {
		t.Fatal("db selection not handled")
	}
	if contains(buttonLabels(api.lastKeyboard().InlineKeyboard), "📨 Invia") {
		t.Fatal("Invia must still wait for the boolean field")
	}
	if !ui.handleCallback(callbackFor(t, api.lastKeyboard().InlineKeyboard, "Sì", 0)) {
		t.Fatal("boolean selection not handled")
	}
	labels = buttonLabels(api.lastKeyboard().InlineKeyboard)
	if !contains(labels, "📨 Invia") {
		t.Fatalf("Invia must appear once required fields are answered: %v", labels)
	}
	if !ui.handleCallback(callbackFor(t, api.lastKeyboard().InlineKeyboard, "📨 Invia", 0)) {
		t.Fatal("confirm not handled")
	}
	select {
	case outcome := <-outcomes:
		if outcome.Action != middleware.ElicitationActionAccept {
			t.Fatalf("expected accept, got %q", outcome.Action)
		}
		if outcome.Values["db"] != "postgres" {
			t.Fatalf("answers lost in accumulation: %+v", outcome.Values)
		}
		// The schema says boolean, so the value must reach the agent as a bool.
		if cache, ok := outcome.Values["cache"].(bool); !ok || !cache {
			t.Fatalf("boolean answer must be a JSON bool, got %#v", outcome.Values["cache"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("multi-field form never resolved")
	}
}

// TestTelegramMixedFormIsDeclinableButNotAnswerable documents the deliberate
// limit: a form mixing free text with other fields cannot be completed from
// chat, but the user can always refuse or abort it.
func TestTelegramMixedFormIsDeclinableButNotAnswerable(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	request := middleware.ElicitationRequest{
		ID: "session:sess_1", AgentID: "codex", Mode: middleware.ElicitationModeForm, SessionID: "sess_1",
		Fields: []middleware.ElicitationField{
			{Name: "text", Type: "string", Required: true},
			{Name: "choice", Type: "enum", Required: true, Options: []middleware.ElicitationOption{{Value: "a"}}},
		},
	}
	outcomes := askAsync(t, service, request)
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}
	// Free text is not consumed for mixed forms: the chat message must route.
	if ui.consumeText(4242, "hello") {
		t.Fatal("a mixed form must not hijack chat messages")
	}
	rows := keyboardRows(t, api.sentMessages()[0])
	if !ui.handleCallback(callbackFor(t, rows, "🚫 Rifiuta", 0)) {
		t.Fatal("decline must always be available")
	}
	select {
	case outcome := <-outcomes:
		if outcome.Action != middleware.ElicitationActionDecline {
			t.Fatalf("expected decline, got %q", outcome.Action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("decline never resolved the request")
	}
}

// TestTelegramManyOptionsStayWithinTelegramLimits keeps a huge form from
// producing a keyboard Telegram rejects outright.
func TestTelegramManyOptionsStayWithinTelegramLimits(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	options := make([]middleware.ElicitationOption, 0, 120)
	for i := 0; i < 120; i++ {
		options = append(options, middleware.ElicitationOption{Value: fmt.Sprintf("v%d", i), Label: fmt.Sprintf("Option %d", i)})
	}
	request := middleware.ElicitationRequest{
		ID: "session:sess_1", Mode: middleware.ElicitationModeForm, SessionID: "sess_1",
		Fields: []middleware.ElicitationField{{Name: "pick", Type: "enum", Required: true, Options: options}},
	}
	outcomes := askAsync(t, service, request)
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}
	rows := keyboardRows(t, api.sentMessages()[0])
	buttons := 0
	for _, row := range rows {
		for _, button := range row {
			buttons++
			if button.CallbackData == nil {
				t.Fatalf("button without callback data: %+v", button)
			}
			if len(*button.CallbackData) > 64 {
				t.Fatalf("callback data exceeds the Telegram 64-byte limit: %q", *button.CallbackData)
			}
		}
	}
	if buttons > telegramMaxKeyboardButtons {
		t.Fatalf("keyboard carries %d buttons, above the %d limit", buttons, telegramMaxKeyboardButtons)
	}
	// The truncated keyboard must still be answerable.
	if !ui.handleCallback(callbackFor(t, rows, "Option 0", 0)) {
		t.Fatal("first option must remain tappable")
	}
	ui.handleCallback(callbackFor(t, api.lastKeyboard().InlineKeyboard, "📨 Invia", 0))
	select {
	case <-outcomes:
	case <-time.After(2 * time.Second):
		t.Fatal("truncated keyboard never resolved")
	}
}

// TestTelegramTextRoutingAfterResolution proves a chat is released once its
// request is settled: later messages belong to the agent again.
func TestTelegramTextRoutingAfterResolution(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	request := middleware.ElicitationRequest{
		ID: "session:sess_1", Mode: middleware.ElicitationModeForm, SessionID: "sess_1",
		Fields: []middleware.ElicitationField{{Name: "branch", Type: "string", Required: true}},
	}
	outcomes := askAsync(t, service, request)
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}
	if !ui.consumeText(4242, "feature/x") {
		t.Fatal("text must be consumed while pending")
	}
	ui.handleCallback(callbackFor(t, api.lastKeyboard().InlineKeyboard, "📨 Invia", 0))
	select {
	case outcome := <-outcomes:
		if outcome.Values["branch"] != "feature/x" {
			t.Fatalf("answer lost: %+v", outcome.Values)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("never resolved")
	}
	if ui.consumeText(4242, "another message") {
		t.Fatal("messages must route to the agent once the request is settled")
	}
}

// TestTelegramNumberOverflowIsRejected keeps numeric parsing honest.
func TestTelegramNumberOverflowIsRejected(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	request := middleware.ElicitationRequest{
		ID: "session:sess_1", Mode: middleware.ElicitationModeForm, SessionID: "sess_1",
		Fields: []middleware.ElicitationField{{Name: "n", Type: "number", Required: true}},
	}
	askAsync(t, service, request)
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}
	for _, bad := range []string{"1e999999", "NaN", "Inf", "--1", "1,5", ""} {
		if !ui.consumeText(4242, bad) {
			t.Fatalf("%q must be consumed as an answer attempt", bad)
		}
		if len(service.Pending()) != 1 {
			t.Fatalf("%q must not resolve the request", bad)
		}
	}
	if !ui.consumeText(4242, "42") {
		t.Fatal("a valid number must be consumed")
	}
}

// TestTelegramURLModeHostDisplayHidesCredentials pins the display rule: the
// consent prompt must show the destination host and the full URL, but never a
// password embedded in the authority.
func TestTelegramURLModeHostDisplayHidesCredentials(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	_, api, _ := newTestUI(t, service)
	request := middleware.ElicitationRequest{
		ID: "session:sess_1", AgentID: "claude", Mode: middleware.ElicitationModeURL, SessionID: "sess_1",
		URL: "https://user:secret@auth.example.com/oauth",
	}
	askAsync(t, service, request)
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}
	text := api.sentMessages()[0].Text
	if !strings.Contains(text, "auth.example.com") {
		t.Fatalf("host must be displayed: %q", text)
	}
	if strings.Contains(text, "secret") {
		t.Fatalf("the displayed host must not include credentials: %q", text)
	}
}

// TestTelegramPromptTextIsNotMisinterpreted guards against Markdown/HTML
// injection by agents: message text is sent verbatim, without a parse mode.
func TestTelegramPromptTextIsNotMisinterpreted(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	_, api, _ := newTestUI(t, service)
	request := middleware.ElicitationRequest{
		ID: "session:sess_1", Mode: middleware.ElicitationModeForm, SessionID: "sess_1",
		Message: "<b>bold</b> *star* _under_ `code`",
		Fields:  []middleware.ElicitationField{{Name: "a", Type: "string", Required: true}},
	}
	askAsync(t, service, request)
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}
	sent := api.sentMessages()[0]
	if sent.ParseMode != "" {
		t.Fatalf("prompt must not set a parse mode, got %q", sent.ParseMode)
	}
	if !strings.Contains(sent.Text, "<b>bold</b>") {
		t.Fatalf("agent text must pass through verbatim: %q", sent.Text)
	}
}

// TestTelegramUnknownActionNeverSelectsAnAnswer is the fuzz regression: an
// unknown (or empty) action used to fall through to the option branch and
// silently record option 0 as the answer.
func TestTelegramUnknownActionNeverSelectsAnAnswer(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	askAsync(t, service, choiceRequest())
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}

	// A callback that carries the real token, a valid field and option index,
	// but an action nobody defined.
	rows := keyboardRows(t, api.sentMessages()[0])
	token := callbackToken(t, rows)
	for _, action := range []string{"", "pickk", "approve", "PICK"} {
		if !ui.handleCallback(&tgbotapi.CallbackQuery{
			ID:      "cb",
			Data:    "el:" + token + ":" + action + ":0:0",
			Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 4242}},
		}) {
			t.Fatalf("callback with action %q must be handled", action)
		}
		if len(service.Pending()) != 1 {
			t.Fatalf("action %q resolved the request", action)
		}
		ui.mu.Lock()
		pending := ui.pending[token]
		var recorded interface{}
		if pending != nil {
			for _, field := range pending.request.Fields {
				if value, ok := pending.answers[field.Name]; ok {
					recorded = value
				}
			}
		}
		ui.mu.Unlock()
		if recorded != nil {
			t.Fatalf("action %q recorded an answer: %#v", action, recorded)
		}
	}

	// The legitimate pick must still work afterwards.
	if !ui.handleCallback(callbackFor(t, rows, "Allow once", 0)) {
		t.Fatal("a valid pick must be handled")
	}
	ui.handleCallback(callbackFor(t, api.lastKeyboard().InlineKeyboard, "📨 Invia", 0))
	if len(service.Pending()) != 0 {
		t.Fatal("a valid pick followed by confirm must resolve the request")
	}
}

// keyboardRows extracts the inline keyboard of a sent message, failing the test
// rather than panicking when the shape is unexpected.
func keyboardRows(t *testing.T, message tgbotapi.MessageConfig) [][]tgbotapi.InlineKeyboardButton {
	t.Helper()
	markup, ok := message.ReplyMarkup.(*tgbotapi.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("message does not carry an inline keyboard: %#v", message.ReplyMarkup)
	}
	return markup.InlineKeyboard
}

// callbackToken extracts the opaque token from a rendered keyboard.
func callbackToken(t *testing.T, rows [][]tgbotapi.InlineKeyboardButton) string {
	t.Helper()
	for _, row := range rows {
		for _, button := range row {
			if button.CallbackData == nil {
				continue
			}
			parts := strings.Split(*button.CallbackData, ":")
			if len(parts) == 5 && parts[0] == callbackDataPrefix {
				return parts[1]
			}
		}
	}
	t.Fatal("no elicitation callback data found in the keyboard")
	return ""
}
