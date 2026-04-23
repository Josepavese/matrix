package agentmgr

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/middleware"
)

// AgentConfig represents how to launch and communicate with a specific agent.
type AgentConfig struct {
	Command         string   `json:"command"`
	Args            []string `json:"args"`
	Env             []string `json:"env,omitempty"`
	Protocol        string   `json:"protocol,omitempty"`
	Kind            string   `json:"kind,omitempty"`
	Transport       string   `json:"transport,omitempty"`
	Address         string   `json:"address,omitempty"`
	CardURL         string   `json:"card_url,omitempty"`
	ProtocolVersion string   `json:"protocol_version,omitempty"`
	HealthcheckPath string   `json:"healthcheck_path"`
	EnvIsolation    bool     `json:"env_isolation"`
	Active          *bool    `json:"active,omitempty"`
}

// IsActive returns whether the agent should be considered enabled.
// Missing values default to true by policy.
func (c AgentConfig) IsActive() bool {
	return c.Active == nil || *c.Active
}

// Registry handles loading the SSOT definitions for available agents.
type Registry struct {
	configs map[string]AgentConfig
}

// NewRegistry initializes the registry by loading all agent definitions from the Vault.
func NewRegistry(_ middleware.ConfigReader, store middleware.Storage) (*Registry, error) {
	ids, err := agentcfg.ListAgentIDs(store)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents from vault: %w", err)
	}

	configs := make(map[string]AgentConfig)
	for _, id := range ids {
		entry, err := agentcfg.LoadEntry(store, id)
		if err != nil {
			return nil, err
		}

		// Map Entry (storable) to AgentConfig (runtime).
		cfg := AgentConfig{
			Command:         entry.Config.Command,
			Args:            entry.Config.Args,
			Env:             entry.Config.Env,
			Protocol:        entry.Config.Kind,
			Kind:            entry.Config.Kind,
			Transport:       entry.Config.Transport,
			Address:         entry.Config.Address,
			CardURL:         entry.Config.CardURL,
			ProtocolVersion: entry.Config.ProtocolVersion,
			HealthcheckPath: entry.Config.HealthcheckPath,
			EnvIsolation:    entry.Config.EnvIsolation,
			Active:          entry.Config.Active,
		}
		endpoint := protocolEndpointFromAgentConfig(cfg)
		cfg.Protocol = string(endpoint.Kind)
		configs[id] = cfg

		// Apply user overrides
		if entry.Override.Active != nil {
			configs[id] = func(c AgentConfig) AgentConfig {
				c.Active = entry.Override.Active
				return c
			}(configs[id])
		}
		if len(entry.Override.Env) > 0 {
			configs[id] = func(c AgentConfig) AgentConfig {
				c.Env = append(append([]string{}, c.Env...), entry.Override.Env...)
				return c
			}(configs[id])
		}
	}

	return &Registry{
		configs: configs,
	}, nil
}

// Get finds the configuration for a given agent ID.
func (r *Registry) Get(agentID string) (AgentConfig, error) {
	cfg, ok := r.configs[agentID]
	if !ok {
		return AgentConfig{}, fmt.Errorf("agent '%s' not found in registry", agentID)
	}
	return cfg, nil
}

// List returns all configured agent IDs.
func (r *Registry) List() []string {
	ids := make([]string, 0, len(r.configs))
	for id, cfg := range r.configs {
		if !cfg.IsActive() {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// IDs returns all known agent IDs, including inactive ones.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.configs))
	for id := range r.configs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// SeedFromConfigFile reads agent definitions from a JSON config file and seeds
// missing agents into the vault. This handles pre-installed agents (like opencode)
// that are not installed via the ACP Registry but are available in configs/agents.json.
func SeedFromConfigFile(store middleware.Storage, configReader middleware.ConfigReader, path string) error {
	data, err := configReader.ReadConfig(path)
	if err != nil {
		return fmt.Errorf("failed to read agent config file %s: %w", path, err)
	}

	var configs map[string]AgentConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return fmt.Errorf("failed to parse agent config file %s: %w", path, err)
	}

	for id, cfg := range configs {
		existing, err := agentcfg.LoadEntry(store, id)
		if err != nil {
			continue
		}
		// Skip if already has a command (installed by installer or already seeded)
		if existing.Config.Command != "" {
			continue
		}
		// Seed from config file
		entry := agentcfg.Entry{
			Config: agentcfg.Config{
				Command:         cfg.Command,
				Args:            cfg.Args,
				Env:             cfg.Env,
				Kind:            cfg.Kind,
				Transport:       cfg.Transport,
				Address:         cfg.Address,
				CardURL:         cfg.CardURL,
				ProtocolVersion: cfg.ProtocolVersion,
				HealthcheckPath: cfg.HealthcheckPath,
				EnvIsolation:    cfg.EnvIsolation,
				Active:          cfg.Active,
			},
		}
		if err := agentcfg.SaveEntry(store, id, entry); err != nil {
			return fmt.Errorf("failed to seed agent %s: %w", id, err)
		}
	}
	return nil
}

func protocolEndpointFromAgentConfig(cfg AgentConfig) middleware.ProtocolEndpoint {
	return agentcfg.NormalizeEndpoint(agentcfg.Config{
		Command:         cfg.Command,
		Args:            cfg.Args,
		Env:             cfg.Env,
		Kind:            cfg.Kind,
		Transport:       cfg.Transport,
		Address:         cfg.Address,
		CardURL:         cfg.CardURL,
		ProtocolVersion: cfg.ProtocolVersion,
		HealthcheckPath: cfg.HealthcheckPath,
		EnvIsolation:    cfg.EnvIsolation,
		Active:          cfg.Active,
	})
}
