package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

func (w *Wizard) cachedOrFetchedAgents(state *WizardState) ([]AgentEntry, bool) {
	agentData := state.Context["_agent_list"]
	if agentData == "" {
		agents, err := w.fetchAgentList()
		return agents, err == nil && len(agents) > 0
	}
	var agents []AgentEntry
	if err := json.Unmarshal([]byte(agentData), &agents); err != nil {
		return nil, false
	}
	return agents, len(agents) > 0
}

func selectedAgentFromInput(input string, agents []AgentEntry) (AgentEntry, bool) {
	idx := parseSelectionIndex(input)
	if idx < 1 || idx > len(agents) {
		return AgentEntry{}, false
	}
	return agents[idx-1], true
}

func (w *Wizard) activateSelectedAgent(state *WizardState, selected AgentEntry, channelID string) (string, error) {
	installMsg := fmt.Sprintf(w.localizer.GetString(state.Language, "agent_installing"), selected.Name)
	slog.Info("activating agent from wizard", "agent", selected.ID, "source", selected.Source, "kind", selected.Kind)
	if err := w.activator.Activate(context.Background(), selected); err != nil {
		state.AgentName = ""
		state.Step = 2
		state.Context = map[string]string{"channel_id": channelID}
		return fmt.Sprintf(w.localizer.GetString(state.Language, "agent_install_failed"), selected.Name, err) + "\n\n" + w.promptForStep(*state), nil
	}
	if selected.ID == agentCodex {
		msg, err := w.prepareCodexSelection(state)
		return installMsg + " ✅\n" + msg, err
	}
	return installMsg + " ✅\n" + w.promptForStep(*state), nil
}
