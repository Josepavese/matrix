package agents

import (
	"encoding/json"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/frontendevents"
	"github.com/Josepavese/matrix/internal/middleware"
)

const (
	permissionDecisionApproved = "approved"
	permissionDecisionDenied   = "denied"
)

type permissionAudit struct {
	params           json.RawMessage
	options          interface{}
	title            string
	description      string
	subject          map[string]interface{}
	sessionID        string
	toolCallID       string
	operation        string
	operationKnown   bool
	decision         string
	optionID         string
	denialReason     string
	providerResponse string
	auto             bool
}

// operationState reports the explicit presence contract for the operation: a
// request that could not be identified from structured protocol fields says so
// instead of leaving the reader with an empty operation.
func (a permissionAudit) operationState() string {
	if a.operationKnown && strings.TrimSpace(a.operation) != "" {
		return frontendevents.PermissionOperationKnown
	}
	return frontendevents.PermissionOperationUnknown
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
//
// The structured diagnosis is additive and deterministic: operation and
// operation_state always agree (the state says "unknown" when the request did
// not name one), tool_call_id and session_id correlate the denial, denial_reason
// says which policy refused an approval, and provider_response says what was
// actually returned to the peer. Human-facing text is never used to derive any
// of them.
func permissionAuditMetadata(a permissionAudit) map[string]interface{} {
	metadata := map[string]interface{}{
		"protocol_method":   "session/request_permission",
		"options":           a.options,
		"decision":          a.decision,
		"option_id":         a.optionID,
		"approval_mode":     approvalMode(a.auto),
		"operation_state":   a.operationState(),
		"provider_response": a.providerResponse,
	}
	if operation := strings.TrimSpace(a.operation); operation != "" {
		metadata["operation"] = operation
	}
	if a.toolCallID != "" {
		metadata["tool_call_id"] = a.toolCallID
	}
	if a.sessionID != "" {
		metadata["session_id"] = a.sessionID
	}
	if a.denialReason != "" {
		metadata["denial_reason"] = a.denialReason
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
