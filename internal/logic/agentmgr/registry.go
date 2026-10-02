package agentmgr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentidentity"
	"github.com/Josepavese/matrix/internal/middleware"
)

// AgentConfig is the runtime view of the current governed endpoint config.
type AgentConfig = agentcfg.Config

// Registry handles loading the SSOT definitions for available agents.
//
// It is a view of the vault rather than a copy taken once. An operator enables an
// agent while the daemon is already serving, and a snapshot that predates that
// change is what turned a working agent into a 409 telling them to restart: the
// run path asked whether the agent speaks ACP by consulting a map that had never
// heard of it. So a snapshot older than registryTTL is refreshed before it is
// used, and a lookup that MISSES reloads at once — the miss is itself the proof
// that the view is out of date, and waiting for the TTL would answer with the
// snapshot that caused the problem.
type Registry struct {
	store middleware.Storage

	mu       sync.Mutex
	configs  map[string]AgentConfig
	loadedAt time.Time
}

// registryTTL bounds how long a snapshot may be reused without re-reading the
// vault. It is a variable so a test can make the window observable without
// sleeping through it.
var registryTTL = time.Second

// NewRegistry initializes the registry by loading all agent definitions from the Vault.
func NewRegistry(_ middleware.ConfigReader, store middleware.Storage) (*Registry, error) {
	configs, err := loadAgentConfigs(store)
	if err != nil {
		return nil, err
	}
	return &Registry{store: store, configs: configs, loadedAt: time.Now()}, nil
}

// loadAgentConfigs reads the current definitions from the vault and applies the
// user overrides. The constructor and every reload read the same way, so a
// reloaded view cannot differ from a freshly built one.
func loadAgentConfigs(store middleware.Storage) (map[string]AgentConfig, error) {
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
	return configs, nil
}

// snapshot returns the current view together with the error of the read that
// produced it. A read that fails keeps the previous view AND returns the error
// with it, so a caller that finds what it needs can keep serving, while one that
// does not can say why instead of claiming the agent does not exist.
func (r *Registry) snapshot() (map[string]AgentConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.configs != nil && time.Since(r.loadedAt) < registryTTL {
		return r.configs, nil
	}
	return r.reloadLocked()
}

// reloadLocked re-reads the vault. A definition the daemon cannot read must not
// take away the agents it is already serving, and the next lookup tries again.
// A registry built without a vault has nothing to re-read and serves the view it
// was given.
func (r *Registry) reloadLocked() (map[string]AgentConfig, error) {
	if r.store == nil {
		r.loadedAt = time.Now()
		return r.configs, nil
	}
	configs, err := loadAgentConfigs(r.store)
	if err != nil {
		slog.Warn("agent registry reload failed; serving the previous view",
			"component", "agent_registry", "event", "registry_reload_failed", "error", err)
		return r.configs, err
	}
	r.configs = configs
	r.loadedAt = time.Now()
	return r.configs, nil
}

// Get finds the configuration for a given agent ID.
func (r *Registry) Get(agentID string) (AgentConfig, error) {
	configs, _ := r.snapshot()
	if cfg, ok := configs[agentID]; ok {
		return cfg, nil
	}

	// The agent is not in the view: read the vault once more before answering
	// that it does not exist, so an agent registered seconds ago is served
	// instead of refused with a restart as its remedy.
	r.mu.Lock()
	configs, reloadErr := r.reloadLocked()
	r.mu.Unlock()
	if cfg, ok := configs[agentID]; ok {
		return cfg, nil
	}
	if reloadErr != nil {
		return AgentConfig{}, fmt.Errorf("agent '%s' could not be resolved: %w", agentID, reloadErr)
	}
	return AgentConfig{}, fmt.Errorf("agent '%s' not found in registry%s", agentID, agentidentity.PublicAgentIDHint(agentID))
}

// List returns all configured agent IDs.
func (r *Registry) List() []string {
	configs, _ := r.snapshot()
	ids := make([]string, 0, len(configs))
	for id, cfg := range configs {
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
	configs, _ := r.snapshot()
	ids := make([]string, 0, len(configs))
	for id := range configs {
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
