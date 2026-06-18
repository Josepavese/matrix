package agents

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
)

type permissionRequestOption struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind"`
}

func (h *defaultRequestHandler) handlePermissionRequest(_ context.Context, log *slog.Logger, params json.RawMessage) (interface{}, error) {
	var req struct {
		Options []permissionRequestOption `json:"options"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		log.Warn("failed to parse permission request", "error", err)
		if h.isTrustMode() {
			h.notifyPermission(permissionAudit{params: params, decision: "approved", optionID: "allow-once", auto: true})
			return h.approveResponse("allow-once"), nil
		}
		h.notifyPermission(permissionAudit{params: params, decision: "denied"})
		return h.denyResponse(nil), nil
	}

	if !h.isTrustMode() {
		log.Info("denying permission (trust mode off)", "event", "permission_denied", "options_count", len(req.Options))
		h.notifyPermission(permissionAudit{params: params, options: req.Options, decision: "denied"})
		return h.denyResponse(req.Options), nil
	}

	optionID := "allow-once"
	for _, opt := range req.Options {
		if opt.Kind == "allow_once" || opt.Kind == "allow_always" {
			optionID = opt.OptionID
			break
		}
	}
	log.Info("auto-approving permission", "event", "permission_approved", "optionID", optionID, "options_count", len(req.Options))
	h.notifyPermission(permissionAudit{params: params, options: req.Options, decision: "approved", optionID: optionID, auto: true})
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
