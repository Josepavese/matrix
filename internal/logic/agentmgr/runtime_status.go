package agentmgr

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/middleware"
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
	// ArtifactVerification is the install-time integrity evidence for the
	// artifact Matrix downloaded from the registry, absent when no install
	// recorded one. It is the machine-readable answer to "was the digest
	// verified?" for consumers of `matrix doctor`.
	ArtifactVerification *agentcfg.ArtifactVerification `json:"artifact_verification,omitempty"`
}

type inspectInput struct {
	AgentID   string
	Config    AgentConfig
	Installed bool
	State     RuntimeState
	Meta      agentcfg.Meta
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
		endpoint := protocolEndpointFromAgentConfig(cfg)
		meta, metaErr := agentcfg.LoadMeta(store, agentID)
		report := buildRuntimeReport(inspectInput{
			AgentID:   agentID,
			Config:    cfg,
			Installed: isInstalledEndpoint(cfg, endpoint, proc),
			State:     states[agentID],
			Meta:      meta,
		}, canDial)
		if metaErr != nil {
			report.Warnings = append(report.Warnings, "agent metadata unavailable: "+metaErr.Error())
		}
		reports = append(reports, report)
		if len(report.Warnings) > 0 {
			warnings = append(warnings, report.AgentID+": "+report.Warnings[0])
		}
	}
	return reports, warnings, nil
}

// RuntimeReportRequest is what a single-agent runtime report needs. It is a
// request rather than a parameter list so a caller cannot silently transpose
// two of the collaborators it has to supply.
type RuntimeReportRequest struct {
	Store    middleware.Storage
	Registry *Registry
	Process  middleware.Process
	CanDial  func(string) bool
	AgentID  string
}

// BuildRuntimeReport generates the runtime report for one agent, so a command
// that shows a single agent reports what the daemon says about it without
// building every other agent's report.
func BuildRuntimeReport(request RuntimeReportRequest) (AgentRuntimeReport, error) {
	if request.Registry == nil {
		return AgentRuntimeReport{}, fmt.Errorf("agent registry not available")
	}
	cfg, err := request.Registry.Get(request.AgentID)
	if err != nil {
		return AgentRuntimeReport{}, err
	}
	states, err := LoadRuntimeStates(request.Store)
	if err != nil {
		return AgentRuntimeReport{}, err
	}
	endpoint := protocolEndpointFromAgentConfig(cfg)
	meta, metaErr := agentcfg.LoadMeta(request.Store, request.AgentID)
	report := buildRuntimeReport(inspectInput{
		AgentID:   request.AgentID,
		Config:    cfg,
		Installed: isInstalledEndpoint(cfg, endpoint, request.Process),
		State:     states[request.AgentID],
		Meta:      meta,
	}, request.CanDial)
	if metaErr != nil {
		report.Warnings = append(report.Warnings, "agent metadata unavailable: "+metaErr.Error())
	}
	return report, nil
}

func buildRuntimeReport(input inspectInput, canDial func(string) bool) AgentRuntimeReport {
	endpoint := protocolEndpointFromAgentConfig(input.Config)
	report := AgentRuntimeReport{
		AgentID:              input.AgentID,
		Protocol:             string(endpoint.Kind),
		Mode:                 runtimeMode(endpoint.Transport),
		Active:               input.Config.IsActive(),
		Installed:            input.Installed,
		Status:               "unknown",
		ArtifactVerification: input.Meta.ArtifactVerification,
	}

	switch {
	case !report.Active:
		report.Status = "inactive"
	case !report.Installed:
		report.Status = "missing_executable"
		report.Warnings = append(report.Warnings, "executable not found in PATH")
	case report.Mode == "on_demand":
		if input.State.Status == "" {
			// Registered and installed, but the runtime has not observed this
			// configuration yet. The status names the operator's situation
			// rather than the absence of a probe: what has to happen next is an
			// apply, and calling it "not probed" left a freshly registered
			// agent reading like a fault.
			report.Status = "pending_apply"
			report.Warnings = append(report.Warnings, "registration recorded, runtime not yet applied: "+
				middleware.RegistrationRemedyThen("re-run this command"))
		} else {
			report = applyRuntimeState(report, input.State, canDial)
		}
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

func isInstalledEndpoint(cfg AgentConfig, endpoint middleware.ProtocolEndpoint, proc middleware.Process) bool {
	if endpoint.Kind == middleware.ProtocolKindA2A && (endpoint.Address != "" || endpoint.CardURL != "") {
		return true
	}
	if cfg.Command == "" {
		return false
	}
	return proc.HasExecutable(cfg.Command)
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
