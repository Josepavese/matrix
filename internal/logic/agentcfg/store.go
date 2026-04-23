package agentcfg

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// KeyPrefix is the vault key prefix for agent configuration entries.
const KeyPrefix = "agent.config."

// Override holds runtime overrides for an agent.
type Override struct {
	Active *bool    `json:"active,omitempty"`
	Env    []string `json:"env,omitempty"`
}

// Config holds an agent's command, args, environment and protocol settings.
type Config struct {
	Command         string   `json:"command"`
	Args            []string `json:"args"`
	Env             []string `json:"env,omitempty"`
	Kind            string   `json:"kind,omitempty"`
	Transport       string   `json:"transport,omitempty"`
	Address         string   `json:"address,omitempty"`
	CardURL         string   `json:"card_url,omitempty"`
	ProtocolVersion string   `json:"protocol_version,omitempty"`
	HealthcheckPath string   `json:"healthcheck_path"`
	EnvIsolation    bool     `json:"env_isolation"`
	Active          *bool    `json:"active,omitempty"`
}

// Entry bundles a Config with its Override for a single agent.
type Entry struct {
	Config   Config   `json:"config"`
	Override Override `json:"override"`
}

// Key returns the vault key for an agent's configuration.
func Key(agentID string) string {
	return KeyPrefix + agentID
}

// LoadEntry reads an agent entry (config + override) from the vault.
func LoadEntry(storage middleware.Storage, agentID string) (Entry, error) {
	var entry Entry
	if storage == nil {
		return entry, nil
	}

	data, err := storage.Get(Key(agentID))
	if err != nil {
		return entry, fmt.Errorf("failed to read agent entry for %s: %w", agentID, err)
	}
	if len(data) == 0 {
		return entry, nil
	}
	if err := json.Unmarshal(data, &entry); err != nil {
		return entry, fmt.Errorf("failed to decode agent entry for %s: %w", agentID, err)
	}
	return entry, nil
}

// SaveEntry persists an agent entry (config + override) to the vault.
func SaveEntry(storage middleware.Storage, agentID string, entry Entry) error {
	if storage == nil {
		return fmt.Errorf("storage not available")
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to encode agent entry for %s: %w", agentID, err)
	}
	if err := storage.Set(Key(agentID), data); err != nil {
		return fmt.Errorf("failed to store agent entry for %s: %w", agentID, err)
	}
	return nil
}

// Load reads only the Override portion for an agent.
func Load(storage middleware.Storage, agentID string) (Override, error) {
	entry, err := LoadEntry(storage, agentID)
	if err != nil {
		return Override{}, err
	}
	return entry.Override, nil
}

// Save updates the Override portion for an agent, preserving the existing Config.
func Save(storage middleware.Storage, agentID string, override Override) error {
	entry, err := LoadEntry(storage, agentID)
	if err != nil {
		return err
	}
	entry.Override = override
	return SaveEntry(storage, agentID, entry)
}

// DeleteEntry removes an agent's configuration entry from the vault.
func DeleteEntry(storage middleware.Storage, agentID string) error {
	if storage == nil {
		return fmt.Errorf("storage not available")
	}
	if err := storage.Delete(Key(agentID)); err != nil {
		return fmt.Errorf("failed to delete agent entry for %s: %w", agentID, err)
	}
	return nil
}

// ListAgentIDs returns all agent IDs that have configuration entries.
func ListAgentIDs(storage middleware.Storage) ([]string, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage not available")
	}
	keys, err := storage.List(KeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent overrides: %w", err)
	}
	ids := make([]string, 0, len(keys))
	for _, key := range keys {
		ids = append(ids, strings.TrimPrefix(key, KeyPrefix))
	}
	sort.Strings(ids)
	return ids, nil
}

// UpsertEnv inserts or updates an env entry in the slice.
func UpsertEnv(envs []string, key, value string) []string {
	prefix := key + "="
	replaced := false
	result := make([]string, 0, len(envs)+1)
	for _, env := range envs {
		if strings.HasPrefix(env, prefix) {
			if !replaced {
				result = append(result, prefix+value)
				replaced = true
			}
			continue
		}
		result = append(result, env)
	}
	if !replaced {
		result = append(result, prefix+value)
	}
	return result
}

// RemoveEnv removes all env entries matching the given key.
func RemoveEnv(envs []string, key string) []string {
	prefix := key + "="
	result := make([]string, 0, len(envs))
	for _, env := range envs {
		if strings.HasPrefix(env, prefix) {
			continue
		}
		result = append(result, env)
	}
	return result
}
