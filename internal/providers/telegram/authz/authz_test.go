package authz

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const adminID = int64(777)

func newTestLogger(t *testing.T) (*slog.Logger, *recordHandler) {
	t.Helper()
	handler := &recordHandler{}
	return slog.New(handler), handler
}

func TestNewResolvesConfiguredAdmins(t *testing.T) {
	list, err := New([]int64{adminID, 888})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, id := range []int64{adminID, 888} {
		if _, ok := list[id]; !ok {
			t.Fatalf("configured admin %d missing from the allow-list", id)
		}
	}
	if _, ok := list[999]; ok {
		t.Fatal("unlisted user id must not be authorised")
	}
}

func TestNewRefusesEmptyAdmins(t *testing.T) {
	for _, admins := range [][]int64{nil, {}} {
		list, err := New(admins)
		if err == nil {
			t.Fatalf("New(%v) must fail closed", admins)
		}
		if list != nil {
			t.Fatalf("New(%v) returned an allow-list for empty admins", admins)
		}
		if !strings.Contains(err.Error(), "admins") {
			t.Fatalf("error must name the admins list, got %q", err)
		}
	}
}

// TestAuthorizeCoversEveryUpdateShape walks the shapes the gateway dispatches,
// so a shape added later cannot quietly bypass the check.
func TestAuthorizeCoversEveryUpdateShape(t *testing.T) {
	list, err := New([]int64{adminID})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, tc := range []struct {
		name   string
		update tgbotapi.Update
		want   bool
	}{
		{"private message from admin", tgbotapi.Update{Message: messageFrom(adminID)}, true},
		{"private message from stranger", tgbotapi.Update{Message: messageFrom(999)}, false},
		{"group message from admin", tgbotapi.Update{Message: messageFrom(adminID)}, true},
		{"group message from stranger", tgbotapi.Update{Message: messageFrom(999)}, false},
		{"callback from admin", tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{ID: "cb", From: &tgbotapi.User{ID: adminID}}}, true},
		{"callback from stranger", tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{ID: "cb", From: &tgbotapi.User{ID: 999}}}, false},
		{"edited message from admin", tgbotapi.Update{EditedMessage: messageFrom(adminID)}, true},
		{"edited message from stranger", tgbotapi.Update{EditedMessage: messageFrom(999)}, false},
		{"message without sender", tgbotapi.Update{Message: &tgbotapi.Message{MessageID: 1}}, false},
		{"callback without sender", tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{ID: "cb"}}, false},
		{"channel post", tgbotapi.Update{ChannelPost: &tgbotapi.Message{MessageID: 1}}, false},
		{"unknown shape", tgbotapi.Update{UpdateID: 42}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log, _ := newTestLogger(t)
			if got := list.Authorize(log, tc.update); got != tc.want {
				t.Fatalf("Authorize = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAuthorizeLogsRejectionsWithUserID proves the refusal reaches an operator:
// the level is warn and the sender id is on the record.
func TestAuthorizeLogsRejectionsWithUserID(t *testing.T) {
	list, err := New([]int64{adminID})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log, handler := newTestLogger(t)

	if list.Authorize(log, tgbotapi.Update{Message: messageFrom(999)}) {
		t.Fatal("stranger must not be authorised")
	}
	if ids := handler.userIDsAtWarn(); len(ids) != 1 || ids[0] != 999 {
		t.Fatalf("rejection must log user_id=999 at warn, got %v", ids)
	}

	if !list.Authorize(log, tgbotapi.Update{Message: messageFrom(adminID)}) {
		t.Fatal("admin must be authorised")
	}
	if warnings := handler.warnMessages(); len(warnings) != 1 {
		t.Fatalf("authorised update must not warn, got %v", warnings)
	}
}

// TestZeroAllowListRefusesEveryone pins the fail-closed zero value: a gateway
// built without a resolved allow-list answers nobody.
func TestZeroAllowListRefusesEveryone(t *testing.T) {
	var list AllowList
	log, handler := newTestLogger(t)
	if list.Authorize(log, tgbotapi.Update{Message: messageFrom(adminID)}) {
		t.Fatal("a zero allow-list must authorise nobody")
	}
	if !handler.warned("non-administrator") {
		t.Fatalf("zero allow-list rejection must be logged, got %v", handler.warnMessages())
	}
}

// TestAuthorizeRefusesUnattributableUpdate keeps the no-sender case loud and
// distinct, so an anonymous group administrator is visible in the logs.
func TestAuthorizeRefusesUnattributableUpdate(t *testing.T) {
	list, err := New([]int64{adminID})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log, handler := newTestLogger(t)

	if list.Authorize(log, tgbotapi.Update{}) {
		t.Fatal("an update with no sender must not be authorised")
	}
	if !handler.warned("no identifiable sender") {
		t.Fatalf("unattributable update must be logged, got %v", handler.warnMessages())
	}
}

func messageFrom(id int64) *tgbotapi.Message {
	return &tgbotapi.Message{
		MessageID: 1,
		From:      &tgbotapi.User{ID: id},
		Text:      "hello",
		Chat:      &tgbotapi.Chat{ID: 5, Type: "private"},
	}
}

// recordHandler captures the log records the allow-list emitted.
type recordHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Clone())
	return nil
}

func (h *recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordHandler) warnMessages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var messages []string
	for _, record := range h.records {
		if record.Level >= slog.LevelWarn {
			messages = append(messages, record.Message)
		}
	}
	return messages
}

func (h *recordHandler) warned(fragment string) bool {
	for _, message := range h.warnMessages() {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func (h *recordHandler) userIDsAtWarn() []int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	var ids []int64
	for _, record := range h.records {
		if record.Level < slog.LevelWarn {
			continue
		}
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "user_id" {
				ids = append(ids, attr.Value.Int64())
			}
			return true
		})
	}
	return ids
}
