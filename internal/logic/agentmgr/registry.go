package agentmgr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentidentity"
	"github.com/Josepavese/matrix/internal/middleware"
)

// AgentConfig is the runtime view of the current governed endpoint config.
type AgentConfig = agentcfg.Config

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

		cfg := entry.Config
		cfg.Args = append([]string{}, cfg.Args...)
		cfg.Env = append([]string{}, cfg.Env...)
		cfg.Headers = agentcfg.CloneHeaders(cfg.Headers)

		// Apply user overrides
		if entry.Override.Active != nil {
			cfg.Active = entry.Override.Active
		}
		if len(entry.Override.Env) > 0 {
			cfg.Env = append(cfg.Env, entry.Override.Env...)
		}
		if len(entry.Override.AppendArgs) > 0 {
			cfg.Args = append(cfg.Args, entry.Override.AppendArgs...)
		}
		if err := agentidentity.ValidateRuntimeDefinition(id, cfg.Command, cfg.Args); err != nil {
			return nil, err
		}
		configs[id] = cfg
	}

	return &Registry{configs: configs}, nil
}

// Get finds the configuration for a given agent ID.
func (r *Registry) Get(agentID string) (AgentConfig, error) {
	cfg, ok := r.configs[agentID]
	if !ok {
		return AgentConfig{}, fmt.Errorf("agent '%s' not found in registry%s", agentID, agentidentity.PublicAgentIDHint(agentID))
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
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&configs); err != nil {
		if strings.Contains(err.Error(), `unknown field "protocol"`) {
			return fmt.Errorf("retired agent config field %q; use %q: %w", "protocol", "kind", err)
		}
		return fmt.Errorf("failed to parse agent config file %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("failed to parse agent config file %s: trailing JSON data", path)
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
				Headers:         agentcfg.CloneHeaders(cfg.Headers),
				Tenant:          cfg.Tenant,
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
		Headers:         agentcfg.CloneHeaders(cfg.Headers),
		Tenant:          cfg.Tenant,
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
