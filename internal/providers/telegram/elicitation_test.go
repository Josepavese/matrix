package telegram

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
)

// fakeTelegram records API calls. The registry publishes events from whichever
// goroutine runs Ask, so the fake is mutex-guarded like the real client.
type fakeTelegram struct {
	mu     sync.Mutex
	sent   []tgbotapi.MessageConfig
	edited []tgbotapi.EditMessageTextConfig
	acked  []string
	nextID int
}

func (f *fakeTelegram) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	message, ok := c.(tgbotapi.MessageConfig)
	if !ok {
		return tgbotapi.Message{}, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, message)
	f.nextID++
	return tgbotapi.Message{MessageID: f.nextID, Chat: &tgbotapi.Chat{ID: message.ChatID}}, nil
}

func (f *fakeTelegram) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch value := c.(type) {
	case tgbotapi.EditMessageTextConfig:
		f.edited = append(f.edited, value)
	case tgbotapi.CallbackConfig:
		f.acked = append(f.acked, value.CallbackQueryID)
	}
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func (f *fakeTelegram) sentMessages() []tgbotapi.MessageConfig {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]tgbotapi.MessageConfig(nil), f.sent...)
}

func (f *fakeTelegram) editedMessages() []tgbotapi.EditMessageTextConfig {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]tgbotapi.EditMessageTextConfig(nil), f.edited...)
}

func (f *fakeTelegram) lastKeyboard() *tgbotapi.InlineKeyboardMarkup {
	edited := f.editedMessages()
	if len(edited) == 0 {
		return nil
	}
	return edited[len(edited)-1].ReplyMarkup
}

// waitForSent blocks until at least one prompt was sent or the deadline passes.
func (f *fakeTelegram) waitForSent(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(f.sentMessages()) > 0 {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

func (f *fakeTelegram) waitForEdited(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(f.editedMessages()) > 0 {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

func newTestUI(t *testing.T, service *elicitation.Service) (*elicitationUI, *fakeTelegram, *sessionChatIndex) {
	t.Helper()
	api := &fakeTelegram{}
	chats := newSessionChatIndex()
	chats.bind("sess_1", 4242)
	ui := newElicitationUI(api, service, chats)
	// Production subscribes in Bot.Start; the tests wire it explicitly.
	t.Cleanup(ui.subscribe())
	return ui, api, chats
}

func choiceRequest() middleware.ElicitationRequest {
	return middleware.ElicitationRequest{
		ID:        "session:sess_1",
		AgentID:   "codex",
		Mode:      middleware.ElicitationModeForm,
		Message:   "Allow the mocksrv MCP server to run tool \"choose_database\"?",
		SessionID: "sess_1",
		Fields: []middleware.ElicitationField{{
			Name: "persist", Title: "Approval scope", Type: "enum", Required: true,
			Default: "once",
			Options: []middleware.ElicitationOption{
				{Value: "once", Label: "Allow once"},
				{Value: "always", Label: "Allow and don't ask again"},
			},
		}},
	}
}

// TestTelegramElicitationChoiceFlow covers the live-proven case: an approval
// with labelled options is rendered as buttons and answered through the shared
// registry, with an explicit Invia step so the user reviews before sending.
func TestTelegramElicitationChoiceFlow(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)

	outcomes := make(chan middleware.ElicitationOutcome, 1)
	go func() { outcomes <- service.Ask(context.Background(), choiceRequest()) }()

	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent to the chat")
	}
	sent := api.sentMessages()
	if len(sent) != 1 {
		t.Fatalf("expected exactly one prompt, got %d", len(sent))
	}
	prompt := sent[0]
	if prompt.ChatID != 4242 {
		t.Fatalf("prompt went to the wrong chat: %d", prompt.ChatID)
	}
	if !strings.Contains(prompt.Text, "codex") || !strings.Contains(prompt.Text, "Approval scope") {
		t.Fatalf("prompt must name the agent and the field: %q", prompt.Text)
	}
	rows := keyboardRows(t, prompt)
	labels := buttonLabels(rows)
	if !contains(labels, "Allow once") || !containsLabel(labels, "Rifiuta") || !containsLabel(labels, "Annulla") {
		t.Fatalf("prompt must offer the options and explicit decline/cancel controls: %v", labels)
	}
	if contains(labels, "📨 Invia") {
		t.Fatal("Invia must not appear before the required field is answered")
	}

	// No confirmation yet: the first tap only selects.
	if !ui.handleCallback(callbackFor(t, rows, "Allow once", 0)) {
		t.Fatal("option callback must be handled")
	}
	if len(service.Pending()) != 1 {
		t.Fatal("selection alone must not resolve the request")
	}
	labels = buttonLabels(api.lastKeyboard().InlineKeyboard)
	if !contains(labels, "📨 Invia") {
		t.Fatalf("Invia must appear once the required field is answered: %v", labels)
	}

	if !ui.handleCallback(callbackFor(t, api.lastKeyboard().InlineKeyboard, "📨 Invia", 0)) {
		t.Fatal("confirm callback must be handled")
	}
	select {
	case outcome := <-outcomes:
		if outcome.Action != middleware.ElicitationActionAccept || outcome.Values["persist"] != "once" {
			t.Fatalf("unexpected outcome: %+v", outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("confirming never resolved the registry request")
	}
	if len(service.Pending()) != 0 {
		t.Fatal("answered request must leave the registry")
	}
}

// TestTelegramElicitationDeclineAndCancel keeps the negative path explicit.
func TestTelegramElicitationDeclineAndCancel(t *testing.T) {
	for _, tc := range []struct {
		label  string
		action string
		want   string
	}{
		{"🚫 Rifiuta", "decline", middleware.ElicitationActionDecline},
		{"✖️ Annulla", "cancel", middleware.ElicitationActionCancel},
	} {
		service := elicitation.NewService(time.Minute)
		ui, api, _ := newTestUI(t, service)
		outcomes := make(chan middleware.ElicitationOutcome, 1)
		go func() { outcomes <- service.Ask(context.Background(), choiceRequest()) }()
		if !api.waitForSent(time.Second) {
			t.Fatalf("%s: prompt was not sent", tc.action)
		}
		rows := keyboardRows(t, api.sentMessages()[0])
		if !ui.handleCallback(callbackFor(t, rows, tc.label, 0)) {
			t.Fatalf("%s: callback not handled", tc.action)
		}
		select {
		case outcome := <-outcomes:
			if outcome.Action != tc.want {
				t.Fatalf("%s: expected %s, got %s", tc.action, tc.want, outcome.Action)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: never resolved", tc.action)
		}
	}
}

// TestTelegramElicitationFreeTextReply covers string fields: the next chat
// message is consumed as the answer instead of being routed to the agent.
func TestTelegramElicitationFreeTextReply(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	outcomes := make(chan middleware.ElicitationOutcome, 1)
	request := middleware.ElicitationRequest{
		ID: "session:sess_1", AgentID: "codex", Mode: middleware.ElicitationModeForm,
		Message: "What should the branch be called?", SessionID: "sess_1",
		Fields: []middleware.ElicitationField{{Name: "branch", Type: "string", Required: true}},
	}
	go func() { outcomes <- service.Ask(context.Background(), request) }()
	if !api.waitForSent(time.Second) {
		t.Fatal("free-text prompt was not sent")
	}
	if !strings.Contains(api.sentMessages()[0].Text, "Rispondi con un messaggio") {
		t.Fatalf("free-text prompt must tell the user to reply: %q", api.sentMessages()[0].Text)
	}
	if !ui.consumeText(4242, "feature/elicitation") {
		t.Fatal("text reply must be consumed by the pending elicitation")
	}
	if ui.consumeText(9999, "unrelated") {
		t.Fatal("a chat without a pending elicitation must not consume messages")
	}
	labels := buttonLabels(api.lastKeyboard().InlineKeyboard)
	if !contains(labels, "📨 Invia") {
		t.Fatalf("text answer must unlock Invia: %v", labels)
	}
	if !ui.handleCallback(callbackFor(t, api.lastKeyboard().InlineKeyboard, "📨 Invia", 0)) {
		t.Fatal("confirm callback must be handled")
	}
	select {
	case outcome := <-outcomes:
		if outcome.Action != middleware.ElicitationActionAccept || outcome.Values["branch"] != "feature/elicitation" {
			t.Fatalf("unexpected outcome: %+v", outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("text answer never resolved the request")
	}
}

// TestTelegramElicitationExpiryRetractsPrompt proves the registry remains the
// source of truth: when the request is resolved elsewhere, the chat prompt is
// retracted instead of staying tappable.
func TestTelegramElicitationExpiryRetractsPrompt(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	go service.Ask(context.Background(), choiceRequest())
	if !api.waitForSent(time.Second) {
		t.Fatal("prompt was not sent")
	}
	// Simulate the HTTP surface (or a timeout) resolving it first.
	if !service.Respond("session:sess_1", middleware.DeclineElicitation()) {
		t.Fatal("registry refused the external answer")
	}
	if !api.waitForEdited(time.Second) {
		t.Fatal("resolved request must retract the chat prompt")
	}
	edited := api.editedMessages()
	last := edited[len(edited)-1]
	if !strings.Contains(last.Text, "rifiutata") {
		t.Fatalf("retraction must state the outcome: %q", last.Text)
	}
	if ui.handleCallback(&tgbotapi.CallbackQuery{ID: "cb-late", Data: "el:e1:pick:0:0"}) != true {
		t.Fatal("late callback must still be acknowledged")
	}
}

// TestTelegramElicitationUnknownChatIsSkipped keeps cross-channel safety: a
// request from a session this bot does not own is never rendered here.
func TestTelegramElicitationUnknownChatIsSkipped(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	request := choiceRequest()
	request.SessionID = "sess_other"
	go service.Ask(context.Background(), request)
	time.Sleep(50 * time.Millisecond)
	if sent := api.sentMessages(); len(sent) != 0 {
		t.Fatalf("a foreign session must not be rendered in this chat: %+v", sent)
	}
	_ = ui
}

// TestTelegramElicitationURLConsent covers URL mode: the host and full URL are
// shown and consent is explicit; Matrix never opens the link itself.
func TestTelegramElicitationURLConsent(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	outcomes := make(chan middleware.ElicitationOutcome, 1)
	request := middleware.ElicitationRequest{
		ID: "session:sess_1", AgentID: "claude", Mode: middleware.ElicitationModeURL,
		Message: "Please authorize access to your repositories.", SessionID: "sess_1",
		URL: "https://accounts.example.com/oauth/authorize?x=1",
	}
	go func() { outcomes <- service.Ask(context.Background(), request) }()
	if !api.waitForSent(time.Second) {
		t.Fatal("url prompt was not sent")
	}
	text := api.sentMessages()[0].Text
	if !strings.Contains(text, "accounts.example.com") || !strings.Contains(text, "https://accounts.example.com/oauth/authorize?x=1") {
		t.Fatalf("url mode must show host and full url: %q", text)
	}
	if !strings.Contains(text, "non apre il link") {
		t.Fatalf("url mode must state that Matrix does not open the link: %q", text)
	}
	urlRows := keyboardRows(t, api.sentMessages()[0])
	labels := buttonLabels(urlRows)
	if !contains(labels, "✅ Consento ad aprirlo") {
		t.Fatalf("url mode must require explicit consent: %v", labels)
	}
	if !ui.handleCallback(callbackFor(t, urlRows, "✅ Consento ad aprirlo", 0)) {
		t.Fatal("consent callback must be handled")
	}
	select {
	case outcome := <-outcomes:
		if outcome.Action != middleware.ElicitationActionAccept {
			t.Fatalf("consent must accept url mode, got %s", outcome.Action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("consent never resolved the request")
	}
}

// TestTelegramElicitationNumberValidation rejects values the schema forbids.
func TestTelegramElicitationNumberValidation(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, api, _ := newTestUI(t, service)
	request := middleware.ElicitationRequest{
		ID: "session:sess_1", Mode: middleware.ElicitationModeForm, SessionID: "sess_1",
		Fields: []middleware.ElicitationField{{Name: "retries", Type: "number", Required: true}},
	}
	go service.Ask(context.Background(), request)
	if !api.waitForSent(time.Second) {
		t.Fatal("number prompt was not sent")
	}
	if !ui.consumeText(4242, "not-a-number") {
		t.Fatal("invalid reply must still be consumed")
	}
	if len(service.Pending()) != 1 {
		t.Fatal("invalid value must leave the request pending")
	}
	if sent := api.sentMessages(); len(sent) < 2 || !strings.Contains(sent[1].Text, "Valore non valido") {
		t.Fatalf("invalid value must be reported: %+v", sent)
	}
	if !ui.consumeText(4242, "3") {
		t.Fatal("valid reply must be consumed")
	}
}

// --- helpers ---

func buttonLabels(rows [][]tgbotapi.InlineKeyboardButton) []string {
	var labels []string
	for _, row := range rows {
		for _, b := range row {
			labels = append(labels, b.Text)
		}
	}
	return labels
}

// containsLabel matches button text that carries an emoji prefix.
func containsLabel(values []string, target string) bool {
	for _, value := range values {
		if strings.Contains(value, target) {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// callbackFor builds the callback a tap on the given label would produce.
func callbackFor(t *testing.T, rows [][]tgbotapi.InlineKeyboardButton, label string, _ int) *tgbotapi.CallbackQuery {
	t.Helper()
	for _, row := range rows {
		for _, b := range row {
			if b.Text == label {
				data := ""
				if b.CallbackData != nil {
					data = *b.CallbackData
				}
				return &tgbotapi.CallbackQuery{
					ID:   "cb-" + label,
					Data: data,
					Message: &tgbotapi.Message{
						Chat: &tgbotapi.Chat{ID: 4242},
					},
				}
			}
		}
	}
	t.Fatalf("button %q not found in keyboard", label)
	return nil
}
