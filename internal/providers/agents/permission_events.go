package agents

import (
	"encoding/json"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

type permissionAudit struct {
	params      json.RawMessage
	options     interface{}
	title       string
	description string
	subject     map[string]interface{}
	decision    string
	optionID    string
	auto        bool
}

func (h *defaultRequestHandler) notifyPermission(a permissionAudit) {
	h.notifierMu.Lock()
	notifier := h.notifier
	h.notifierMu.Unlock()
	if notifier == nil {
		return
	}
	notifier.OnThought(middleware.ThoughtUpdate{
		Type:     middleware.ThoughtTypePermission,
		Content:  string(a.params),
		Metadata: permissionAuditMetadata(a),
	})
}

// permissionAuditMetadata projects what the request asked for alongside the
// decision. Version 2's title, description and structured subject are what a
// human needs to judge the request, so they are carried through instead of being
// left in the raw payload alone.
func permissionAuditMetadata(a permissionAudit) map[string]interface{} {
	metadata := map[string]interface{}{
		"protocol_method": "session/request_permission",
		"options":         a.options,
		"decision":        a.decision,
		"option_id":       a.optionID,
		"approval_mode":   approvalMode(a.auto),
	}
	if strings.TrimSpace(a.title) != "" {
		metadata["permission_title"] = a.title
	}
	if strings.TrimSpace(a.description) != "" {
		metadata["permission_description"] = a.description
	}
	if len(a.subject) > 0 {
		metadata["permission_subject"] = a.subject
		if kind, ok := a.subject["type"].(string); ok && strings.TrimSpace(kind) != "" {
			metadata["permission_subject_type"] = strings.TrimSpace(kind)
		}
	}
	return metadata
}

func approvalMode(auto bool) string {
	if auto {
		return "auto"
	}
	return "manual_or_policy"
}
