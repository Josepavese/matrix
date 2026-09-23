// Package authz decides which Telegram updates a bot may act on.
//
// The Telegram gateway answers whoever can reach the bot, and its commands
// include agent delegation and system tools, so the sender must be checked
// against the configured admin list before any update is routed. This package
// owns that decision: it is a positive allow-list over the configured user ids,
// and every shape it cannot attribute to a listed user is refused.
package authz

import (
	"errors"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// AllowList is the resolved set of Telegram user ids permitted to drive a bot.
// The zero value authorises nobody: a gateway constructed without one refuses
// every update instead of accepting everyone.
type AllowList map[int64]struct{}

// New resolves the configured admins into an allow-list. It fails closed on an
// empty list so an enabled channel cannot silently accept the whole platform.
func New(admins []int64) (AllowList, error) {
	if len(admins) == 0 {
		return nil, errors.New("telegram admins list is empty: refusing to build a gateway anyone can drive")
	}
	list := make(AllowList, len(admins))
	for _, id := range admins {
		list[id] = struct{}{}
	}
	return list, nil
}

// Authorize reports whether the update comes from an administrator, logging
// every rejection with the sender id so an operator can see who was turned
// away. Updates with no identifiable human sender fail closed, which covers
// channel posts and messages sent on behalf of a group.
func (a AllowList) Authorize(log *slog.Logger, update tgbotapi.Update) bool {
	user, ok := updateSender(update)
	if !ok {
		log.Warn("dropping telegram update with no identifiable sender", "event", "update_rejected", "reason", "no_sender")
		return false
	}
	if _, allowed := a[user.ID]; !allowed {
		log.Warn("dropping telegram update from non-administrator", "event", "update_rejected", "reason", "not_admin", "user_id", user.ID)
		return false
	}
	return true
}

// updateSender extracts the user whose action the gateway would dispatch, for
// every update shape it handles: callback queries first (the gateway dispatches
// them first), then plain and edited messages. A shape carrying no user returns
// false and is refused by the caller.
func updateSender(update tgbotapi.Update) (*tgbotapi.User, bool) {
	switch {
	case update.CallbackQuery != nil:
		return update.CallbackQuery.From, update.CallbackQuery.From != nil
	case update.Message != nil:
		return update.Message.From, update.Message.From != nil
	case update.EditedMessage != nil:
		return update.EditedMessage.From, update.EditedMessage.From != nil
	default:
		return nil, false
	}
}
