package agentmgr

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
)

const runtimeStatePrefix = "runtime.agent."

// RuntimeState holds the persisted runtime status for a single agent.
type RuntimeState struct {
	AgentID   string    `json:"agent_id"`
	Protocol  string    `json:"protocol"`
	Mode      string    `json:"mode"`
	Status    string    `json:"status"`
	Address   string    `json:"address,omitempty"`
	Port      int       `json:"port,omitempty"`
	PID       int       `json:"pid,omitempty"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentRuntimeReport is a diagnostic report for a single agent's runtime state.
type AgentRuntimeReport struct {
	AgentID   string    `json:"agent_id"`
	Protocol  string    `json:"protocol"`
	Mode      string    `json:"mode"`
	Active    bool      `json:"active"`
	Installed bool      `json:"installed"`
	Status    string    `json:"status"`
	Address   string    `json:"address,omitempty"`
	PID       int       `json:"pid,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	Warnings  []string  `json:"warnings,omitempty"`
}

type inspectInput struct {
	AgentID   string
	Config    AgentConfig
	Installed bool
	State     RuntimeState
}

func runtimeStateKey(agentID string) string {
	return runtimeStatePrefix + agentID
}

// SaveRuntimeState persists a RuntimeState entry to the vault.
func SaveRuntimeState(store middleware.Storage, state RuntimeState) error {
	state.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return store.Set(runtimeStateKey(state.AgentID), data)
}

// LoadRuntimeStates loads all agent runtime states from the vault.
func LoadRuntimeStates(store middleware.Storage) (map[string]RuntimeState, error) {
	keys, err := store.List(runtimeStatePrefix)
	if err != nil {
		return nil, err
	}
	states := make(map[string]RuntimeState, len(keys))
	for _, key := range keys {
		data, err := store.Get(key)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			continue
		}
		var state RuntimeState
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, fmt.Errorf("invalid runtime state for %s: %w", key, err)
		}
		states[state.AgentID] = state
	}
	return states, nil
}

// BuildRuntimeReports generates runtime reports for all registered agents.
func BuildRuntimeReports(store middleware.Storage, reg *Registry, proc middleware.Process, canDial func(string) bool) ([]AgentRuntimeReport, []string, error) {
	states, err := LoadRuntimeStates(store)
	if err != nil {
		return nil, nil, err
	}

	ids := reg.IDs()
	reports := make([]AgentRuntimeReport, 0, len(ids))
	warnings := make([]string, 0, len(ids))
	for _, agentID := range ids {
		cfg, err := reg.Get(agentID)
		if err != nil {
			return nil, nil, err
		}
		report := buildRuntimeReport(inspectInput{
			AgentID:   agentID,
			Config:    cfg,
			Installed: cfg.Command != "" && proc.HasExecutable(cfg.Command),
			State:     states[agentID],
		}, canDial)
		reports = append(reports, report)
		if len(report.Warnings) > 0 {
			warnings = append(warnings, report.AgentID+": "+report.Warnings[0])
		}
	}
	return reports, warnings, nil
}

func buildRuntimeReport(input inspectInput, canDial func(string) bool) AgentRuntimeReport {
	report := AgentRuntimeReport{
		AgentID:   input.AgentID,
		Protocol:  input.Config.Protocol,
		Mode:      runtimeMode(input.Config.Protocol),
		Active:    input.Config.IsActive(),
		Installed: input.Installed,
		Status:    "unknown",
	}

	switch {
	case !report.Active:
		report.Status = "inactive"
	case !report.Installed:
		report.Status = "missing_executable"
		report.Warnings = append(report.Warnings, "executable not found in PATH")
	case report.Mode == "on_demand":
		report.Status = "ready_on_demand"
	default:
		report = applyRuntimeState(report, input.State, canDial)
	}

	if report.Status == "unknown" {
		report.Status = "not_observed"
		report.Warnings = append(report.Warnings, "no runtime state recorded")
	}
	return report
}

func runtimeMode(protocol string) string {
	if protocol == "ws" || protocol == "http" {
		return "supervised"
	}
	return "on_demand"
}

func applyRuntimeState(report AgentRuntimeReport, state RuntimeState, canDial func(string) bool) AgentRuntimeReport {
	if state.Status == "" {
		return report
	}
	report.Address = state.Address
	report.PID = state.PID
	report.UpdatedAt = state.UpdatedAt
	report.Status = state.Status
	if state.Error != "" {
		report.Warnings = append(report.Warnings, state.Error)
	}
	if state.Status == "running" && state.Address != "" && !canDial(state.Address) {
		report.Status = "unreachable"
		report.Warnings = append(report.Warnings, "recorded runtime endpoint is not reachable")
	}
	return report
}
