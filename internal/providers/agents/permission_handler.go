package agents

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
)

type permissionRequestOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// permissionRequest is the request both generations put on the wire. Version 2
// kept optionId, name and kind, and replaced version 1's toolCall field with a
// required prompt title, an optional description and a structured subject, so a
// handler that reads only options cannot say what is being approved.
type permissionRequest struct {
	SessionID   string                    `json:"sessionId"`
	Title       string                    `json:"title"`
	Description string                    `json:"description"`
	Subject     map[string]interface{}    `json:"subject"`
	Options     []permissionRequestOption `json:"options"`
	ToolCall    struct {
		ToolCallID string `json:"toolCallId"`
		Title      string `json:"title"`
		Kind       string `json:"kind"`
	} `json:"toolCall"`
}

// permissionTitle is the human-readable subject of the request in either
// generation's shape: version 2 sends it directly, version 1 sent it on the tool
// call the permission belongs to.
func (r permissionRequest) permissionTitle() string {
	if title := strings.TrimSpace(r.Title); title != "" {
		return title
	}
	return strings.TrimSpace(r.ToolCall.Title)
}

// permissionSubjectType names the operation a version 2 subject describes — a
// tool call, a command, or a future variant — for logging and projection.
func (r permissionRequest) permissionSubjectType() string {
	kind, _ := r.Subject["type"].(string)
	return strings.TrimSpace(kind)
}

func (h *defaultRequestHandler) handlePermissionRequest(_ context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	var req permissionRequest
	if err := json.Unmarshal(params, &req); err != nil {
		log.Warn("failed to parse permission request", "error", err)
		if h.isTrustMode() {
			h.notifyPermission(permissionAudit{params: params, decision: "approved", optionID: "allow-once", auto: true})
			return h.approveResponse("allow-once"), nil
		}
		h.notifyPermission(permissionAudit{params: params, decision: "denied"})
		return h.denyResponse(nil), nil
	}
	audit := permissionAudit{
		params:      params,
		options:     req.Options,
		title:       req.permissionTitle(),
		description: req.Description,
		subject:     req.Subject,
	}
	if !h.isTrustMode() {
		log.Info("denying permission (trust mode off)",
			"event", "permission_denied", "options_count", len(req.Options),
			"permission_title", audit.title, "permission_subject_type", req.permissionSubjectType())
		audit.decision = "denied"
		h.notifyPermission(audit)
		return h.denyResponse(req.Options), nil
	}

	optionID := "allow-once"
	for _, opt := range req.Options {
		if opt.Kind == "allow_once" || opt.Kind == "allow_always" {
			optionID = opt.OptionID
			break
		}
	}
	log.Info("auto-approving permission",
		"event", "permission_approved", "optionID", optionID, "options_count", len(req.Options),
		"permission_title", audit.title, "permission_subject_type", req.permissionSubjectType())
	audit.decision, audit.optionID, audit.auto = "approved", optionID, true
	h.notifyPermission(audit)
	return h.approveResponse(optionID), nil
}

func (h *defaultRequestHandler) approveResponse(optionID string) map[string]interface{} {
	return map[string]interface{}{
		"outcome": map[string]interface{}{
			"outcome":  "selected",
			"optionId": optionID,
		},
	}
}

func (h *defaultRequestHandler) denyResponse(options []permissionRequestOption) map[string]interface{} {
	for _, opt := range options {
		if strings.HasPrefix(opt.Kind, "reject") && opt.OptionID != "" {
			return map[string]interface{}{
				"outcome": map[string]interface{}{
					"outcome":  "selected",
					"optionId": opt.OptionID,
				},
			}
		}
	}
	return map[string]interface{}{
		"outcome": map[string]interface{}{
			"outcome": "cancelled",
		},
	}
}
