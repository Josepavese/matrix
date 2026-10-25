package agents

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/frontendevents"
	"github.com/Josepavese/matrix/pkg/zedacp"
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
		ToolCallID string                 `json:"toolCallId"`
		Title      string                 `json:"title"`
		Kind       string                 `json:"kind"`
		RawInput   map[string]interface{} `json:"rawInput"`
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

// permissionOperation derives the operation identity from the request's
// structured protocol fields. It never reads free text: a title or description
// is prompt copy, not an operation, and guessing from prose would make the
// diagnosis depend on the peer's wording. When no structured source names the
// operation the caller records it as explicitly unknown.
func (r permissionRequest) permissionOperation() (string, bool) {
	// Version 2's command subject carries the exact execution as a required
	// structured command; a tool-call subject carries the tool call patch.
	if operation := frontendevents.OperationFromStructured(r.Subject); operation != "" {
		return operation, true
	}
	if toolCall, ok := r.Subject["toolCall"].(map[string]interface{}); ok {
		if operation := frontendevents.OperationFromStructured(toolCall); operation != "" {
			return operation, true
		}
		if operation := operationFromToolKind(stringValue(toolCall["kind"])); operation != "" {
			return operation, true
		}
	}
	// Version 1's tool call names the tool kind, and the raw input it was
	// invoked with when the peer provides it.
	if operation := frontendevents.OperationFromStructured(r.ToolCall.RawInput); operation != "" {
		return operation, true
	}
	if operation := operationFromToolKind(r.ToolCall.Kind); operation != "" {
		return operation, true
	}
	return "", false
}

// permissionToolCallID correlates the permission with the tool call it belongs.
// Version 2 puts it on the command subject or inside the tool-call subject;
// version 1 puts it beside the tool call title.
func (r permissionRequest) permissionToolCallID() string {
	if id := stringValue(r.Subject["toolCallId"]); id != "" {
		return id
	}
	if toolCall, ok := r.Subject["toolCall"].(map[string]interface{}); ok {
		if id := stringValue(toolCall["toolCallId"]); id != "" {
			return id
		}
	}
	return strings.TrimSpace(r.ToolCall.ToolCallID)
}

// operationFromToolKind accepts an ACP tool kind as a category-level operation
// identity when the request names no exact command. The field is a protocol
// enum, not prose, so it is safe to report; a value that is not a plain token is
// rejected instead of guessed at.
func operationFromToolKind(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" || strings.ContainsAny(kind, `/\ `) {
		return ""
	}
	return kind
}

// stringValue reads a structured string field, refusing any other JSON type
// instead of formatting it into a synthetic identity.
func stringValue(value interface{}) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func (h *defaultRequestHandler) handlePermissionRequest(_ context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	var req permissionRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, zedacp.NewInvalidParamsError("permission")
	}
	audit := permissionAudit{
		params:      params,
		options:     req.Options,
		title:       req.permissionTitle(),
		description: req.Description,
		subject:     req.Subject,
		sessionID:   strings.TrimSpace(req.SessionID),
		toolCallID:  req.permissionToolCallID(),
	}
	audit.operation, audit.operationKnown = req.permissionOperation()
	if !h.isTrustMode() {
		log.Info("denying permission (trust mode off)",
			"event", "permission_denied", "options_count", len(req.Options),
			"permission_title", audit.title, "permission_subject_type", req.permissionSubjectType(),
			"operation", audit.operation, "operation_state", audit.operationState())
		audit.decision = permissionDecisionDenied
		audit.denialReason = frontendevents.PermissionDenialTrustModeOff
		response, providerResponse := h.denyResponse(req.Options)
		audit.providerResponse = providerResponse
		h.notifyPermission(audit)
		return response, nil
	}

	optionID := ""
	for _, opt := range req.Options {
		if opt.Kind == "allow_once" || opt.Kind == "allow_always" {
			optionID = opt.OptionID
			break
		}
	}
	if optionID == "" {
		audit.decision = permissionDecisionDenied
		audit.denialReason = frontendevents.PermissionDenialNoAllowOption
		response, providerResponse := h.denyResponse(req.Options)
		audit.providerResponse = providerResponse
		h.notifyPermission(audit)
		return response, nil
	}
	log.Info("auto-approving permission",
		"event", "permission_approved", "optionID", optionID, "options_count", len(req.Options),
		"permission_title", audit.title, "permission_subject_type", req.permissionSubjectType(),
		"operation", audit.operation, "operation_state", audit.operationState())
	audit.decision, audit.optionID, audit.auto = permissionDecisionApproved, optionID, true
	audit.providerResponse = frontendevents.PermissionResponseSelected
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

// denyResponse answers a denied request and reports the provider response it
// represents: a selected reject option when the peer offered one, or the
// protocol's cancelled outcome when it offered none.
func (h *defaultRequestHandler) denyResponse(options []permissionRequestOption) (map[string]interface{}, string) {
	for _, opt := range options {
		if strings.HasPrefix(opt.Kind, "reject") && opt.OptionID != "" {
			return map[string]interface{}{
				"outcome": map[string]interface{}{
					"outcome":  "selected",
					"optionId": opt.OptionID,
				},
			}, frontendevents.PermissionResponseSelected
		}
	}
	return map[string]interface{}{
		"outcome": map[string]interface{}{
			"outcome": "cancelled",
		},
	}, frontendevents.PermissionResponseCancelled
}
