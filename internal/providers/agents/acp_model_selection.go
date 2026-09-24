package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (c *acpConversationClient) selectTurnModel(ctx context.Context, sessionID, modelID, fallbackModelID string) (middleware.ModelSelection, error) {
	selection, err := c.applyRequestedModel(ctx, sessionID, modelID)
	if err == nil || fallbackModelID == "" || !isModelSelectionRejection(err) {
		return selection, err
	}
	fallback, fallbackErr := c.applyRequestedModel(ctx, sessionID, fallbackModelID)
	if fallbackErr != nil {
		return middleware.ModelSelection{}, fmt.Errorf("requested model failed: %w; authorized fallback failed: %w", err, fallbackErr)
	}
	fallback.FallbackUsed = true
	fallback.FallbackReason = "requested_model_unavailable"
	return fallback, nil
}

func isModelSelectionRejection(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	if isNonModelFailure(lower) {
		return false
	}
	return strings.Contains(lower, "model") && hasModelRejectionPhrase(lower)
}

func isNonModelFailure(lower string) bool {
	return isProviderAuthError(lower) || strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "connection") || strings.Contains(lower, "method not found") ||
		strings.Contains(lower, "does not support session/set_model")
}

func hasModelRejectionPhrase(lower string) bool {
	for _, phrase := range []string{"unavailable", "unknown model option", "not found", "does not exist", "unsupported", "rejected", "not have access"} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func (c *acpConversationClient) applyRequestedModel(ctx context.Context, sessionID, modelID string) (middleware.ModelSelection, error) {
	if modelID == "" {
		return middleware.ModelSelection{}, nil
	}
	selection := middleware.ModelSelection{ConfiguredModel: modelID, Verification: "unverified"}
	if c.negotiatedProtocolVersion() < 2 {
		resp, err := c.currentACPClient().SetConfigOption(ctx, acpSetConfigOptionRequest{
			SessionID: sessionID, ConfigID: "model", Value: modelID,
		})
		if err != nil {
			return middleware.ModelSelection{}, fmt.Errorf("requested model %q was not accepted by provider: %w", modelID, err)
		}
		if resp == nil {
			return middleware.ModelSelection{}, fmt.Errorf("provider did not confirm requested model %q", modelID)
		}
		for _, option := range resp.ConfigOptions {
			if option.ID == "model" && option.Current == modelID {
				selection.EffectiveModel = option.Current
				selection.Verification = "provider_confirmed"
				return selection, nil
			}
		}
		return middleware.ModelSelection{}, fmt.Errorf("provider did not confirm requested model %q", modelID)
	}
	setter, ok := c.currentACPClient().(interface {
		SetSessionModel(context.Context, acpSetSessionModelRequest) (*acpSetSessionModelResponse, error)
	})
	if !ok {
		return middleware.ModelSelection{}, fmt.Errorf("ACP adapter does not support session/set_model")
	}
	resp, err := setter.SetSessionModel(ctx, acpSetSessionModelRequest{SessionID: sessionID, ModelID: modelID})
	if err != nil {
		return middleware.ModelSelection{}, fmt.Errorf("requested model %q was not accepted by provider: %w", modelID, err)
	}
	if resp == nil {
		return middleware.ModelSelection{}, fmt.Errorf("provider did not acknowledge requested model %q", modelID)
	}
	return selection, nil
}
