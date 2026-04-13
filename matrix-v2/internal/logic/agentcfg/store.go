package agentcfg

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jose/matrix-v2/internal/middleware"
)

const KeyPrefix = "agent.config."

type Override struct {
	Active *bool    `json:"active,omitempty"`
	Env    []string `json:"env,omitempty"`
}

type Config struct {
	Command         string   `json:"command"`
	Args            []string `json:"args"`
	Env             []string `json:"env,omitempty"`
	Protocol        string   `json:"protocol"`
	HealthcheckPath string   `json:"healthcheck_path"`
	EnvIsolation    bool     `json:"env_isolation"`
	Active          *bool    `json:"active,omitempty"`
}

type Entry struct {
	Config   Config   `json:"config"`
	Override Override `json:"override"`
}

func Key(agentID string) string {
	return KeyPrefix + agentID
}

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

func Load(storage middleware.Storage, agentID string) (Override, error) {
	entry, err := LoadEntry(storage, agentID)
	if err != nil {
		return Override{}, err
	}
	return entry.Override, nil
}

func Save(storage middleware.Storage, agentID string, override Override) error {
	entry, err := LoadEntry(storage, agentID)
	if err != nil {
		return err
	}
	entry.Override = override
	return SaveEntry(storage, agentID, entry)
}

func DeleteEntry(storage middleware.Storage, agentID string) error {
	if storage == nil {
		return fmt.Errorf("storage not available")
	}
	if err := storage.Delete(Key(agentID)); err != nil {
		return fmt.Errorf("failed to delete agent entry for %s: %w", agentID, err)
	}
	return nil
}

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
