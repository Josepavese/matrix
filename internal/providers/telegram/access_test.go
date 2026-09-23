package telegram

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/telegram/authz"
)

// The admin gate is the only thing standing between any Telegram user and the
// operator's agents, so these tests assert what did NOT happen: a rejected
// update must never reach the router, and a rejected button must never reach
// the elicitation registry.

const (
	adminUserID    = int64(1001)
	strangerUserID = int64(1002)
	privateChatID  = int64(2001)
	groupChatID    = int64(2002)
)

type routeCall struct {
	channelID string
	input     string
}

// spyRouter records every Route call. The other SessionRouter methods exist
// only to satisfy the interface: the gateway never calls them on this path.
type spyRouter struct {
	mu    sync.Mutex
	calls []routeCall
}

func (r *spyRouter) Route(_ context.Context, channelID, _ string, input string, _ middleware.ThoughtNotifier) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, routeCall{channelID: channelID, input: input})
	r.mu.Unlock()
	return "routed", nil
}

func (r *spyRouter) routed() []routeCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]routeCall(nil), r.calls...)
}

func (r *spyRouter) HandleSessionAction(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (r *spyRouter) HandleSessionActionTyped(context.Context, middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	return middleware.SessionActionResult{}, nil
}

func (r *spyRouter) HandleWorkspaceAction(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (r *spyRouter) HandleWorkspaceActionTyped(context.Context, middleware.WorkspaceActionRequest) (middleware.WorkspaceActionResult, error) {
	return middleware.WorkspaceActionResult{}, nil
}

func (r *spyRouter) HandleWorkspaceRead(context.Context, string, string, string, int) (string, error) {
	return "", nil
}

func (r *spyRouter) HandleWorkspaceReadTyped(context.Context, middleware.WorkspaceReadRequest) (middleware.WorkspaceReadResult, error) {
	return middleware.WorkspaceReadResult{}, nil
}

func (r *spyRouter) HandleIntent(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (r *spyRouter) HandleIntentTyped(context.Context, middleware.IntentActionRequest) (middleware.IntentActionResult, error) {
	return middleware.IntentActionResult{}, nil
}

// fakeHTTPClient stands in for the Telegram HTTP transport, so a test can drive
// the whole bot without a network and can count every API call it made.
type fakeHTTPClient struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeHTTPClient) Do(*http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	body := `{"ok":true,"result":{"message_id":1,"date":1700000000,"chat":{"id":2001,"type":"private"}}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{},
	}, nil
}

func (f *fakeHTTPClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newFakeAPI() (*tgbotapi.BotAPI, *fakeHTTPClient) {
	client := &fakeHTTPClient{}
	api := &tgbotapi.BotAPI{Token: "test-token", Client: client}
	api.SetAPIEndpoint("http://telegram.test/bot%s/%s")
	return api, client
}

// newAccessBot builds a bot exactly as NewBot does after its token check, with
// the Telegram transport replaced by the fake.
func newAccessBot(t *testing.T, router middleware.SessionRouter, admins []int64) (*Bot, *fakeHTTPClient) {
	t.Helper()
	api, client := newFakeAPI()
	allowed, err := authz.New(admins)
	if err != nil {
		t.Fatalf("authz.New(%v): %v", admins, err)
	}
	return &Bot{
		router: router,
		api:    api,
		stopCh: make(chan struct{}),
		chats:  newSessionChatIndex(),
		admins: allowed,
	}, client
}

// waitForRoute blocks until the router is called, so a positive case proves
// real dispatch instead of racing the gateway goroutine.
func waitForRoute(t *testing.T, router *spyRouter) routeCall {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if calls := router.routed(); len(calls) > 0 {
			return calls[0]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("router was never called for an authorised update")
	return routeCall{}
}

// assertNeverRouted is the teeth of the gate: it fails if the router or the
// Telegram API was touched at all. A dropped update is rejected synchronously,
// so the short wait only guards against a goroutine that should not exist.
func assertNeverRouted(t *testing.T, router *spyRouter, client *fakeHTTPClient) {
	t.Helper()
	time.Sleep(100 * time.Millisecond)
	if calls := router.routed(); len(calls) != 0 {
		t.Fatalf("rejected update reached the router: %+v", calls)
	}
	if calls := client.callCount(); calls != 0 {
		t.Fatalf("rejected update touched the Telegram API %d time(s)", calls)
	}
}

func privateMessage(from *tgbotapi.User, text string) tgbotapi.Update {
	return tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 10,
		From:      from,
		Text:      text,
		Chat:      &tgbotapi.Chat{ID: privateChatID, Type: "private"},
	}}
}

func groupMessage(from *tgbotapi.User, text string) tgbotapi.Update {
	return tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 11,
		From:      from,
		Text:      text,
		Chat:      &tgbotapi.Chat{ID: groupChatID, Type: "supergroup"},
	}}
}

func TestHandleUpdateRoutesAdministratorPrivateMessage(t *testing.T) {
	router := &spyRouter{}
	bot, _ := newAccessBot(t, router, []int64{adminUserID})

	bot.handleUpdate(context.Background(), slog.New(&captureHandler{}),
		privateMessage(&tgbotapi.User{ID: adminUserID}, "deploy the release"))

	call := waitForRoute(t, router)
	if call.channelID != "telegram_2001" || call.input != "deploy the release" {
		t.Fatalf("administrator message was mangled: %+v", call)
	}
}

func TestHandleUpdateRoutesAdministratorGroupMessage(t *testing.T) {
	router := &spyRouter{}
	bot, _ := newAccessBot(t, router, []int64{adminUserID})

	bot.handleUpdate(context.Background(), slog.New(&captureHandler{}),
		groupMessage(&tgbotapi.User{ID: adminUserID}, "status"))

	call := waitForRoute(t, router)
	if call.channelID != "telegram_2002" || call.input != "status" {
		t.Fatalf("administrator group message was mangled: %+v", call)
	}
}

func TestHandleUpdateDropsNonAdministratorPrivateMessage(t *testing.T) {
	router := &spyRouter{}
	bot, client := newAccessBot(t, router, []int64{adminUserID})

	bot.handleUpdate(context.Background(), slog.New(&captureHandler{}),
		privateMessage(&tgbotapi.User{ID: strangerUserID}, "install malware"))

	assertNeverRouted(t, router, client)
}

func TestHandleUpdateDropsNonAdministratorGroupMessage(t *testing.T) {
	router := &spyRouter{}
	bot, client := newAccessBot(t, router, []int64{adminUserID})

	bot.handleUpdate(context.Background(), slog.New(&captureHandler{}),
		groupMessage(&tgbotapi.User{ID: strangerUserID}, "install malware"))

	assertNeverRouted(t, router, client)
}

// TestHandleUpdateDropsMessageWithoutSender keeps the gate closed for updates
// that carry no user at all: a message posted on behalf of a chat (an anonymous
// group administrator) must not be attributed to anyone.
func TestHandleUpdateDropsMessageWithoutSender(t *testing.T) {
	router := &spyRouter{}
	bot, client := newAccessBot(t, router, []int64{adminUserID})
	handler := &captureHandler{}
	update := tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID:  12,
		SenderChat: &tgbotapi.Chat{ID: groupChatID, Type: "supergroup"},
		Text:       "anonymous attempt",
		Chat:       &tgbotapi.Chat{ID: groupChatID, Type: "supergroup"},
	}}

	bot.handleUpdate(context.Background(), slog.New(handler), update)

	assertNeverRouted(t, router, client)
	if !handler.warned("no identifiable sender") {
		t.Fatalf("unattributable update must be logged as rejected, got %v", handler.messages())
	}
}

// TestHandleUpdateDropsEditedMessageFromNonAdministrator covers the edited
// shape: the gateway does not route edits, but the gate still sees them and
// turns the stranger away instead of staying silent.
func TestHandleUpdateDropsEditedMessageFromNonAdministrator(t *testing.T) {
	router := &spyRouter{}
	bot, client := newAccessBot(t, router, []int64{adminUserID})
	handler := &captureHandler{}
	update := tgbotapi.Update{EditedMessage: &tgbotapi.Message{
		MessageID: 10,
		From:      &tgbotapi.User{ID: strangerUserID},
		Text:      "edited by a stranger",
		Chat:      &tgbotapi.Chat{ID: privateChatID, Type: "private"},
	}}

	bot.handleUpdate(context.Background(), slog.New(handler), update)

	assertNeverRouted(t, router, client)
	if ids := handler.warnedUserIDs(); !containsID(ids, strangerUserID) {
		t.Fatalf("edited message from %d must be logged as rejected, got %v", strangerUserID, ids)
	}
}

// TestHandleUpdateIgnoresEditedMessageFromAdministrator pins the unchanged
// behaviour: edits are never re-routed, for anyone.
func TestHandleUpdateIgnoresEditedMessageFromAdministrator(t *testing.T) {
	router := &spyRouter{}
	bot, client := newAccessBot(t, router, []int64{adminUserID})
	update := tgbotapi.Update{EditedMessage: &tgbotapi.Message{
		MessageID: 10,
		From:      &tgbotapi.User{ID: adminUserID},
		Text:      "edited",
		Chat:      &tgbotapi.Chat{ID: privateChatID, Type: "private"},
	}}

	bot.handleUpdate(context.Background(), slog.New(&captureHandler{}), update)

	assertNeverRouted(t, router, client)
}

// TestHandleUpdateLogsRejectedSenderAtWarn proves the operator can see who was
// turned away: dropping is not silent.
func TestHandleUpdateLogsRejectedSenderAtWarn(t *testing.T) {
	router := &spyRouter{}
	bot, client := newAccessBot(t, router, []int64{adminUserID})
	handler := &captureHandler{}

	bot.handleUpdate(context.Background(), slog.New(handler),
		privateMessage(&tgbotapi.User{ID: strangerUserID}, "hello"))

	assertNeverRouted(t, router, client)
	if ids := handler.warnedUserIDs(); !containsID(ids, strangerUserID) {
		t.Fatalf("rejection must log the sender id %d at warn level, got %v", strangerUserID, ids)
	}
}

// TestHandleUpdateDropsNonAdministratorCallback covers the button path: a
// stranger pressing a button on an elicitation prompt must not answer it.
func TestHandleUpdateDropsNonAdministratorCallback(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, telegramAPI, chats := newTestUI(t, service)
	go service.Ask(context.Background(), choiceRequest())
	if !telegramAPI.waitForSent(time.Second) {
		t.Fatal("elicitation prompt was not sent")
	}
	rows := keyboardRows(t, telegramAPI.sentMessages()[0])

	api, _ := newFakeAPI()
	bot := &Bot{
		router:      &spyRouter{},
		api:         api,
		stopCh:      make(chan struct{}),
		chats:       chats,
		elicitation: ui,
		admins:      authz.AllowList{adminUserID: {}},
	}
	callback := callbackFor(t, rows, "Allow once", 0)
	callback.From = &tgbotapi.User{ID: strangerUserID}

	bot.handleUpdate(context.Background(), slog.New(&captureHandler{}), tgbotapi.Update{CallbackQuery: callback})

	if pending := len(service.Pending()); pending != 1 {
		t.Fatalf("stranger callback reached the elicitation registry: %d pending", pending)
	}
	if acked := ackedCallbacks(telegramAPI); len(acked) != 0 {
		t.Fatalf("stranger callback was acknowledged as if handled: %v", acked)
	}
	if edited := telegramAPI.editedMessages(); len(edited) != 0 {
		t.Fatalf("stranger callback edited the prompt: %+v", edited)
	}
}

// TestHandleUpdateProcessesAdministratorCallback is the positive control for
// the button path: an administrator tap still reaches the elicitation UI.
func TestHandleUpdateProcessesAdministratorCallback(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	ui, telegramAPI, chats := newTestUI(t, service)
	go service.Ask(context.Background(), choiceRequest())
	if !telegramAPI.waitForSent(time.Second) {
		t.Fatal("elicitation prompt was not sent")
	}
	rows := keyboardRows(t, telegramAPI.sentMessages()[0])

	api, _ := newFakeAPI()
	bot := &Bot{
		router:      &spyRouter{},
		api:         api,
		stopCh:      make(chan struct{}),
		chats:       chats,
		elicitation: ui,
		admins:      authz.AllowList{adminUserID: {}},
	}
	callback := callbackFor(t, rows, "Allow once", 0)
	callback.From = &tgbotapi.User{ID: adminUserID}

	bot.handleUpdate(context.Background(), slog.New(&captureHandler{}), tgbotapi.Update{CallbackQuery: callback})

	if acked := ackedCallbacks(telegramAPI); len(acked) == 0 {
		t.Fatal("administrator callback never reached the elicitation UI")
	}
}

// TestBotWithoutAllowListDropsEverything proves the zero value fails closed, so
// a bot value built without NewBot can never answer anyone.
func TestBotWithoutAllowListDropsEverything(t *testing.T) {
	router := &spyRouter{}
	api, client := newFakeAPI()
	bot := &Bot{router: router, api: api, stopCh: make(chan struct{}), chats: newSessionChatIndex()}

	bot.handleUpdate(context.Background(), slog.New(&captureHandler{}),
		privateMessage(&tgbotapi.User{ID: adminUserID}, "hello"))

	assertNeverRouted(t, router, client)
}

// TestNewBotRefusesEmptyAdmins makes the fail-closed construction explicit: no
// admins means no bot, and the error says why. Both cases return before the
// Telegram API client is created, so the test needs no network.
func TestNewBotRefusesEmptyAdmins(t *testing.T) {
	for _, admins := range [][]int64{nil, {}} {
		router := &spyRouter{}
		bot, err := NewBot("test-token", router, admins)
		if err == nil {
			t.Fatalf("NewBot accepted empty admins %v", admins)
		}
		if bot != nil {
			t.Fatalf("NewBot returned a bot for empty admins %v", admins)
		}
		if !strings.Contains(err.Error(), "admins") {
			t.Fatalf("empty-admin error must name the admins list, got %q", err)
		}
		if calls := router.routed(); len(calls) != 0 {
			t.Fatalf("refused bot routed %d message(s)", len(calls))
		}
	}
}

// TestNewBotStillRejectsEmptyToken keeps the constructor's existing refusal
// order: a missing token is reported as such, not as an admins problem.
func TestNewBotStillRejectsEmptyToken(t *testing.T) {
	bot, err := NewBot("", &spyRouter{}, []int64{adminUserID})
	if err == nil || bot != nil {
		t.Fatal("NewBot must reject an empty token")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Fatalf("empty-token error must name the token, got %q", err)
	}
}

// captureHandler records what the gateway logged, so the tests can assert the
// rejection is visible to an operator instead of trusting a code path.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Clone())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) warnings() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var warnings []slog.Record
	for _, record := range h.records {
		if record.Level >= slog.LevelWarn {
			warnings = append(warnings, record)
		}
	}
	return warnings
}

func (h *captureHandler) messages() []string {
	var messages []string
	for _, record := range h.warnings() {
		messages = append(messages, record.Message)
	}
	return messages
}

func (h *captureHandler) warned(fragment string) bool {
	for _, message := range h.messages() {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func (h *captureHandler) warnedUserIDs() []int64 {
	var ids []int64
	for _, record := range h.warnings() {
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "user_id" {
				ids = append(ids, attr.Value.Int64())
			}
			return true
		})
	}
	return ids
}

func containsID(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

// ackedCallbacks reads the recording fake's acknowledgements under its lock.
func ackedCallbacks(f *fakeTelegram) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.acked...)
}
