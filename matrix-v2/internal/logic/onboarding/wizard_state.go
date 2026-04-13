package onboarding

import (
	"encoding/json"
	"log/slog"
)

func (w *Wizard) saveState(key string, state WizardState) {
	data, err := json.Marshal(state)
	if err != nil {
		slog.Warn("failed to encode wizard state", "component", "wizard", "event", "state_encode_failed", "key", key, "error", err)
		return
	}
	if err := w.storage.Set(key, data); err != nil {
		slog.Warn("failed to save wizard state", "component", "wizard", "event", "state_save_failed", "key", key, "error", err)
	}
}

// ForceStart resets the wizard state for a channel and returns the first prompt.
func (w *Wizard) ForceStart(channelID string) (string, error) {
	stateKey := "wizard.state." + channelID
	state := WizardState{
		Step:    0,
		Context: make(map[string]string),
	}
	w.saveState(stateKey, state)

	// Since Process handles Step 0 -> Step 1 transition, we can just call Process with empty input.
	return w.Process(channelID, "")
}
