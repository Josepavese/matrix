package agents

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
)

// This file owns the ACP session mode contract and its evidence: how a launch
// declaration becomes a mode applied to a new or resumed session, how the
// provider's own statement confirms it, and how that confirmation is recorded
// without touching the model attestation. The adapter's session lifecycle and
// the auto-approve defaults stay in acp_adapter.go.

// acpSessionModeContract is the session mode an ACP connection must enforce on
// every session it creates or resumes.
//
// A required mode comes from the neutral MATRIX_ACP_SESSION_MODE launch contract:
// it is applied and then confirmed by a provider statement, and a session whose
// provider cannot confirm it fails before the prompt. The non-required form is
// the Codex launch-policy contract's existing preference, which keeps its
// behaviour: applied through the same surfaces, a provider refusal fails the
// turn, and no extra read-back is demanded of a provider that did not declare
// the neutral contract.
type acpSessionModeContract struct {
	id       string
	required bool
}

// sessionModeContractFor reads the mode contract the endpoint declares. The
// neutral contract is checked first because ResolveEndpoint refuses a launch
// that declares both; a direct adapter construction cannot turn a two-contract
// launch into a silent choice.
func sessionModeContractFor(endpoint middleware.ProtocolEndpoint) acpSessionModeContract {
	if mode := agentlaunch.RequiredSessionMode(endpoint); mode != "" {
		return acpSessionModeContract{id: mode, required: true}
	}
	if mode := agentlaunch.PreferredSessionMode(endpoint); mode != "" {
		return acpSessionModeContract{id: mode}
	}
	return acpSessionModeContract{}
}

// applyConfiguredSessionMode applies the mode after session new or resume.
//
// It selects the surface the provider actually exposes: a config option with
// category/id "mode" (whose set response carries session state), or the version
// 1 session-mode surface. A provider that advertises neither the mode nor a way
// to set it fails closed here, which is before the prompt is sent.
func (c *acpConversationClient) applyConfiguredSessionMode(ctx context.Context, session *middleware.NewSessionResponse, sessionID string) error {
	contract := c.sessionMode
	if session == nil {
		return fmt.Errorf("configured provider mode %q cannot be verified: empty session state", contract.id)
	}
	if option := sessionModeConfigOption(session); option != nil && configOptionOffersMode(option, contract.id) {
		return c.applyConfiguredConfigMode(ctx, sessionID, option, contract)
	}
	if currentSessionMode(session) == contract.id || hasSessionMode(session, contract.id) {
		return c.applyConfiguredSessionModeID(ctx, session, sessionID, contract)
	}
	return fmt.Errorf("configured provider mode %q is unavailable in ACP session state", contract.id)
}

// sessionModeConfigOption finds the option that selects the session mode. Both
// the category and the identifier are honoured because providers have shipped
// each spelling, and the option is the only place the config-option surface
// names its values.
func sessionModeConfigOption(session *middleware.NewSessionResponse) *middleware.ConfigOption {
	for i := range session.ConfigOptions {
		option := &session.ConfigOptions[i]
		if option.Category == "mode" || strings.EqualFold(option.ID, "mode") {
			return option
		}
	}
	return nil
}

func configOptionOffersMode(option *middleware.ConfigOption, mode string) bool {
	if option.Current == mode {
		return true
	}
	for _, value := range option.Options {
		if value.ID == mode {
			return true
		}
	}
	return false
}

// applyConfiguredConfigMode applies and, for a required contract, confirms the
// mode through the config-option surface. The provider's set response states the
// option's current value, so that value is the confirmation: a response that
// states another mode is a contradiction and fails closed, and a response that
// states no mode leaves the application unconfirmed.
func (c *acpConversationClient) applyConfiguredConfigMode(ctx context.Context, sessionID string, option *middleware.ConfigOption, contract acpSessionModeContract) error {
	if option.Current == contract.id {
		if contract.required {
			c.recordSessionModeEvidence(acpSessionModeEvidence{
				ConfiguredMode: contract.id,
				EffectiveMode:  option.Current,
				ConfigID:       option.ID,
				EvidenceSource: sessionModeSourceSessionState,
			})
		}
		return nil
	}
	resp, err := c.currentACPClient().SetConfigOption(ctx, acpSetConfigOptionRequest{SessionID: sessionID, ConfigID: option.ID, Value: contract.id})
	if err != nil {
		return wrapConfiguredModeError(contract.id, err)
	}
	if !contract.required {
		return nil
	}
	evidence, err := confirmSessionModeConfigOption(resp, option.ID, contract.id)
	if err != nil {
		return err
	}
	c.recordSessionModeEvidence(evidence)
	return nil
}

// confirmSessionModeConfigOption reads the provider's confirmation of the mode
// from the set_config_option response. Only the provider's own statement can
// confirm it; the requested value is never reused as evidence.
func confirmSessionModeConfigOption(resp *acpSetConfigOptionResponse, configID, mode string) (acpSessionModeEvidence, error) {
	if resp == nil {
		return acpSessionModeEvidence{}, fmt.Errorf("configured provider mode %q was applied but the provider returned no session state to confirm it", mode)
	}
	for _, option := range resp.ConfigOptions {
		if option.ID != configID {
			continue
		}
		if option.Current == mode {
			return acpSessionModeEvidence{
				ConfiguredMode: mode,
				EffectiveMode:  option.Current,
				ConfigID:       configID,
				EvidenceSource: sessionModeSourceSetConfigOption,
			}, nil
		}
		return acpSessionModeEvidence{}, fmt.Errorf("provider did not confirm configured provider mode %q: session reports %q", mode, option.Current)
	}
	return acpSessionModeEvidence{}, fmt.Errorf("provider did not confirm configured provider mode %q: the response carries no %q option", mode, configID)
}

// applyConfiguredSessionModeID applies the mode through the version 1
// session-mode surface. A required contract is confirmed from the
// current_mode_update the provider reports after the change, because the
// session/set_mode acknowledgement carries no session state; a provider that
// states nothing within the bounded window leaves the mode unconfirmed and fails
// closed.
func (c *acpConversationClient) applyConfiguredSessionModeID(ctx context.Context, session *middleware.NewSessionResponse, sessionID string, contract acpSessionModeContract) error {
	if currentSessionMode(session) == contract.id {
		if contract.required {
			c.recordSessionModeEvidence(acpSessionModeEvidence{
				ConfiguredMode: contract.id,
				EffectiveMode:  contract.id,
				EvidenceSource: sessionModeSourceSessionState,
			})
		}
		return nil
	}
	if !contract.required {
		if err := c.currentACPClient().SetMode(ctx, sessionID, contract.id); err != nil {
			return wrapConfiguredModeError(contract.id, err)
		}
		return nil
	}
	return c.applyAndConfirmSessionModeID(ctx, sessionID, contract.id)
}

// applyAndConfirmSessionModeID registers for the provider's mode update before
// applying it, so a provider that reports the new current mode immediately is
// not missed, and then waits for that statement.
//
// The observation is bounded: a provider that says nothing is not confirmation,
// and a required contract must not be reported as applied on the strength of a
// request the protocol only acknowledged.
func (c *acpConversationClient) applyAndConfirmSessionModeID(ctx context.Context, sessionID, mode string) error {
	watcher, ok := c.currentACPClient().(acpSessionWatcher)
	if !ok {
		return fmt.Errorf("configured provider mode %q cannot be confirmed: the ACP client cannot observe session updates", mode)
	}
	observer := newSessionModeUpdateObserver()
	stop := watcher.WatchSession(sessionID, observer)
	defer stop()
	if err := c.currentACPClient().SetMode(ctx, sessionID, mode); err != nil {
		return wrapConfiguredModeError(mode, err)
	}
	timer := time.NewTimer(acpSessionModeConfirmationWindow)
	defer timer.Stop()
	select {
	case <-observer.done:
		observed, _ := observer.observedMode()
		if observed != mode {
			return fmt.Errorf("provider did not confirm configured provider mode %q: session reports %q", mode, observed)
		}
		c.recordSessionModeEvidence(acpSessionModeEvidence{
			ConfiguredMode: mode,
			EffectiveMode:  observed,
			EvidenceSource: sessionModeSourceCurrentModeUpdate,
		})
		return nil
	case <-timer.C:
		return fmt.Errorf("provider did not confirm configured provider mode %q: no current mode update was observed after session/set_mode", mode)
	case <-ctx.Done():
		return fmt.Errorf("confirm configured provider mode %q: %w", mode, ctx.Err())
	}
}

// sessionModeUpdateObserver captures the first current mode a provider reports
// after the adapter changed it. It records the observed value, not the requested
// one, so a contradictory update is a mismatch rather than a confirmation.
type sessionModeUpdateObserver struct {
	done     chan struct{}
	mu       sync.Mutex
	observed string
}

func newSessionModeUpdateObserver() *sessionModeUpdateObserver {
	return &sessionModeUpdateObserver{done: make(chan struct{})}
}

func (o *sessionModeUpdateObserver) OnUpdate(notification acpSessionNotification) {
	if notification.Update.SessionUpdate != "current_mode_update" {
		return
	}
	mode := strings.TrimSpace(notification.Update.CurrentModeID)
	if mode == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.observed != "" {
		return
	}
	o.observed = mode
	close(o.done)
}

func (o *sessionModeUpdateObserver) observedMode() (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.observed, o.observed != ""
}

// sessionModeEvidence is what the provider stated about the mode this connection
// applied. It exists only when the provider stated it: the adapter records no
// evidence for a request it merely sent.
type acpSessionModeEvidence struct {
	ConfiguredMode string
	EffectiveMode  string
	ConfigID       string
	EvidenceSource string
}

const (
	// sessionModeMetaKey is the metadata key the adapter publishes: the session
	// metadata surface, never the model attestation.
	sessionModeMetaKey = "session_mode"
	// sessionModeSourceSessionState means the session the provider returned
	// already stated the required mode, so nothing had to be changed.
	sessionModeSourceSessionState = "session_state"
	// sessionModeSourceSetConfigOption is the set_config_option response.
	sessionModeSourceSetConfigOption = "session/set_config_option"
	// sessionModeSourceCurrentModeUpdate is the current_mode_update the provider
	// reported after session/set_mode.
	sessionModeSourceCurrentModeUpdate = "session/set_mode+current_mode_update"
)

// acpSessionModeConfirmationWindow bounds the wait for a provider statement
// after session/set_mode. It is a variable so tests can bound the wait without
// changing what production waits for.
var acpSessionModeConfirmationWindow = 2 * time.Second

func (c *acpConversationClient) recordSessionModeEvidence(evidence acpSessionModeEvidence) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.modeEvidence = &evidence
}

func (c *acpConversationClient) currentSessionModeEvidence() *acpSessionModeEvidence {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.modeEvidence == nil {
		return nil
	}
	evidence := *c.modeEvidence
	return &evidence
}

// withSessionModeMetadata attaches the provider-confirmed session mode evidence
// to a turn's metadata. It is separate from the model attestation on purpose: a
// mode is not a model, and recording one must not make a claim about the other.
func (c *acpConversationClient) withSessionModeMetadata(metadata middleware.ConversationMetadata) middleware.ConversationMetadata {
	evidence := c.currentSessionModeEvidence()
	if evidence == nil {
		return metadata
	}
	if metadata.Meta == nil {
		metadata.Meta = map[string]interface{}{}
	}
	metadata.Meta[sessionModeMetaKey] = map[string]interface{}{
		"configured_mode": evidence.ConfiguredMode,
		"effective_mode":  evidence.EffectiveMode,
		"config_id":       evidence.ConfigID,
		"evidence_source": evidence.EvidenceSource,
		"verification":    middleware.ModelVerificationConfirmed,
		"enforcement":     "required",
	}
	return metadata
}

// sessionModeMetadata is the same evidence for a path that materializes a
// session without running a turn. Materialization has no turn metadata of its
// own, so the caller gets a metadata value whose only key is the mode evidence.
func (c *acpConversationClient) sessionModeMetadata() middleware.ConversationMetadata {
	return c.withSessionModeMetadata(middleware.ConversationMetadata{})
}

func currentSessionMode(session *middleware.NewSessionResponse) string {
	if session.Modes == nil {
		return ""
	}
	return session.Modes.CurrentModeID
}

func hasSessionMode(session *middleware.NewSessionResponse, preferred string) bool {
	if session.Modes == nil {
		return false
	}
	for _, mode := range session.Modes.AvailableModes {
		if mode.ID == preferred {
			return true
		}
	}
	return false
}

func wrapConfiguredModeError(mode string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("apply configured provider mode %q: %w", mode, err)
}
