// Package agentcfg manages agent configuration metadata stored in the vault.
package agentcfg

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// MetaKeyPrefix is the vault key prefix for agent metadata entries.
const MetaKeyPrefix = "agent.meta."

// Meta holds display metadata for an agent (name, description, etc.).
// Stored separately from Config to avoid bloating the runtime-critical Config struct.
type Meta struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	Website     string   `json:"website,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	License     string   `json:"license,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	DistTypes   []string `json:"dist_types,omitempty"`
}

// MetaKey returns the vault key for an agent's metadata.
func MetaKey(agentID string) string {
	return MetaKeyPrefix + agentID
}

// SaveMeta persists agent metadata in the vault.
func SaveMeta(storage middleware.Storage, agentID string, meta Meta) error {
	if storage == nil {
		return fmt.Errorf("storage not available")
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to encode meta for %s: %w", agentID, err)
	}
	if err := storage.Set(MetaKey(agentID), data); err != nil {
		return fmt.Errorf("failed to store meta for %s: %w", agentID, err)
	}
	return nil
}

// LoadMeta reads agent metadata from the vault.
// Returns an empty Meta if no metadata is stored.
func LoadMeta(storage middleware.Storage, agentID string) (Meta, error) {
	if storage == nil {
		return Meta{}, nil
	}
	data, err := storage.Get(MetaKey(agentID))
	if err != nil {
		return Meta{}, fmt.Errorf("failed to read meta for %s: %w", agentID, err)
	}
	if len(data) == 0 {
		return Meta{}, nil
	}
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Meta{}, fmt.Errorf("failed to decode meta for %s: %w", agentID, err)
	}
	return meta, nil
}

// DeleteMeta removes agent metadata from the vault.
func DeleteMeta(storage middleware.Storage, agentID string) error {
	if storage == nil {
		return fmt.Errorf("storage not available")
	}
	if err := storage.Delete(MetaKey(agentID)); err != nil {
		return fmt.Errorf("failed to delete meta for %s: %w", agentID, err)
	}
	return nil
}

// ListMetaIDs returns all agent IDs that have metadata stored.
func ListMetaIDs(storage middleware.Storage) ([]string, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage not available")
	}
	keys, err := storage.List(MetaKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list meta keys: %w", err)
	}
	ids := make([]string, 0, len(keys))
	for _, key := range keys {
		ids = append(ids, strings.TrimPrefix(key, MetaKeyPrefix))
	}
	sort.Strings(ids)
	return ids, nil
}
