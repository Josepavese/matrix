package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// sessionOrigin records how the remote session reached the current turn. It is
// read from what the adapter actually did, never inferred: created, resumed and
// loaded mean a provider response carrying session state arrived on this turn,
// reused means the session was already attached in this process and the
// provider was not asked for its state again, and unknown means the
// materialization path could not be classified.
type sessionOrigin string

const (
	sessionOriginUnknown sessionOrigin = ""
	sessionOriginCreated sessionOrigin = "created"
	sessionOriginResumed sessionOrigin = "resumed"
	sessionOriginLoaded  sessionOrigin = "loaded"
	sessionOriginReused  sessionOrigin = "reused"
)

const (
	acpModelConfigOptionID   = "model"
	acpSetConfigOptionMethod = "session/set_config_option"
	acpSetSessionModelMethod = "session/set_model"
)

// turnModelSelection is the model contract of one turn: what the caller asked
// for, the authorized fallback, and how the remote session reached this turn.
type turnModelSelection struct {
	SessionID       string
	ModelID         string
	FallbackModelID string
	Origin          sessionOrigin
}

// selectTurnModelFor applies the requested model and reports what the provider
// stated about it. The session origin is what lets a reused session say the
// verification was not repeated instead of blaming the provider.
func (c *acpConversationClient) selectTurnModelFor(ctx context.Context, req turnModelSelection) (middleware.ModelSelection, error) {
	selection, err := c.applyRequestedModel(ctx, req.SessionID, req.ModelID, req.Origin)
	if err == nil || req.FallbackModelID == "" || !isModelSelectionRejection(err) {
		return selection, err
	}
	fallback, fallbackErr := c.applyRequestedModel(ctx, req.SessionID, req.FallbackModelID, req.Origin)
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

func (c *acpConversationClient) applyRequestedModel(ctx context.Context, sessionID, modelID string, origin sessionOrigin) (middleware.ModelSelection, error) {
	if modelID == "" {
		return middleware.ModelSelection{}, nil
	}
	if c.negotiatedProtocolVersion() < 2 {
		return c.applyModelConfigOption(ctx, sessionID, modelID, origin)
	}
	return c.applySessionModel(ctx, sessionID, modelID, origin)
}

// applyModelConfigOption selects the model through the version 1 session
// configuration surface. Its response reports the session's options with their
// current values, so the provider itself can confirm the selection; a response
// that states nothing leaves the selection unverified with a named reason
// instead of failing a turn the provider accepted.
func (c *acpConversationClient) applyModelConfigOption(ctx context.Context, sessionID, modelID string, origin sessionOrigin) (middleware.ModelSelection, error) {
	resp, err := c.currentACPClient().SetConfigOption(ctx, acpSetConfigOptionRequest{
		SessionID: sessionID, ConfigID: acpModelConfigOptionID, Value: modelID,
	})
	if err != nil {
		return middleware.ModelSelection{}, fmt.Errorf("requested model %q was not accepted by provider: %w", modelID, err)
	}
	if resp == nil {
		return middleware.ModelSelection{}, fmt.Errorf("provider did not confirm requested model %q", modelID)
	}
	return selectionFromEvidence(modelID, configOptionEvidence(resp.ConfigOptions, acpSetConfigOptionMethod), origin)
}

// applySessionModel selects the model through the version 2 session model
// surface. That acknowledgement carries no model identity by protocol, so this
// surface can apply a selection but can never attest it: the run records that
// limitation rather than treating the applied model as confirmed.
func (c *acpConversationClient) applySessionModel(ctx context.Context, sessionID, modelID string, origin sessionOrigin) (middleware.ModelSelection, error) {
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
	return selectionFromEvidence(modelID, sessionStateEvidence{Source: acpSetSessionModelMethod}, origin)
}

// sessionStateEvidence is what one provider response stated about the session
// configuration. StateObserved separates "the provider said nothing" from "the
// provider said there is no model selector": only the second is a fact about
// the session, and the two never report the same reason.
type sessionStateEvidence struct {
	Source        string
	StateObserved bool
	ModelOption   bool
	ModelValue    string
}

func configOptionEvidence(options []acpConfigOption, source string) sessionStateEvidence {
	evidence := sessionStateEvidence{Source: source, StateObserved: len(options) > 0}
	for _, option := range options {
		if option.ID != acpModelConfigOptionID {
			continue
		}
		evidence.ModelOption = true
		evidence.ModelValue = strings.TrimSpace(option.Current)
		break
	}
	return evidence
}

// unverifiedReason names why a selection could not be confirmed. The order is
// deliberate: what the provider stated about the session beats what it
// structurally cannot state, and a reused session that received no session
// state at all is reported as a verification that was not repeated.
func (e sessionStateEvidence) unverifiedReason(origin sessionOrigin) string {
	switch {
	case e.ModelOption && e.ModelValue == "":
		return middleware.ModelUnverifiedEvidenceLost
	case e.StateObserved && !e.ModelOption:
		return middleware.ModelUnverifiedModelNotSelectable
	case origin == sessionOriginReused:
		return middleware.ModelUnverifiedNotRepeated
	default:
		return middleware.ModelUnverifiedProviderDoesNotAttest
	}
}

// selectionFromEvidence turns the provider's own statement into the run's model
// record. A confirmation is never derived from the requested model: when the
// provider states the model, that value is the confirmation; when it states a
// different one, the selection is a contradiction and the turn fails closed;
// when it states nothing, the selection stays unverified with a named reason so
// a consumer can tell a provider that does not attest from a session whose
// verification was not repeated.
func selectionFromEvidence(modelID string, evidence sessionStateEvidence, origin sessionOrigin) (middleware.ModelSelection, error) {
	selection := middleware.ModelSelection{
		ConfiguredModel: modelID,
		Verification:    middleware.ModelVerificationUnverified,
		EvidenceSource:  evidence.Source,
	}
	if evidence.ModelOption && evidence.ModelValue != "" {
		if evidence.ModelValue != modelID {
			return middleware.ModelSelection{}, fmt.Errorf("provider did not confirm requested model %q: session reports %q", modelID, evidence.ModelValue)
		}
		selection.EffectiveModel = evidence.ModelValue
		selection.Verification = middleware.ModelVerificationConfirmed
		return selection, nil
	}
	selection.VerificationReason = evidence.unverifiedReason(origin)
	return selection, nil
}
