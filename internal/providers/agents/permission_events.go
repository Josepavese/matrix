package agents

import (
	"encoding/json"

	"github.com/jose/matrix-v2/internal/middleware"
)

type permissionAudit struct {
	params   json.RawMessage
	options  interface{}
	decision string
	optionID string
	auto     bool
}

func (h *defaultRequestHandler) notifyPermission(a permissionAudit) {
	h.notifierMu.Lock()
	notifier := h.notifier
	h.notifierMu.Unlock()
	if notifier == nil {
		return
	}
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:    middleware.ThoughtTypePermission,
		Content: string(a.params),
		Metadata: map[string]interface{}{
			"protocol_method": "session/request_permission",
			"options":         a.options,
			"decision":        a.decision,
			"option_id":       a.optionID,
			"approval_mode":   approvalMode(a.auto),
		},
	})
}

func approvalMode(auto bool) string {
	if auto {
		return "auto"
	}
	return "manual_or_policy"
}
