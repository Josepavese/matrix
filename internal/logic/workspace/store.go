// Package workspace manages workspace metadata stored in the vault.
package workspace

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
)

const (
	MetaKeyPrefix          = "workspace.meta."
	PathIndexKeyPrefix     = "workspace.path."
	SessionIndexKeyPrefix  = "workspace.sessions."
	ChannelIndexKeyPrefix  = "workspace.channels."
	maxIndexedAssociations = 20
)

// Meta describes a logical work context known to Matrix.
type Meta struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Kind            string                 `json:"kind,omitempty"`
	RootPath        string                 `json:"root_path,omitempty"`
	RepoURL         string                 `json:"repo_url,omitempty"`
	DefaultBranch   string                 `json:"default_branch,omitempty"`
	Labels          []string               `json:"labels,omitempty"`
	PolicyProfile   string                 `json:"policy_profile,omitempty"`
	DefaultAgentID  string                 `json:"default_agent_id,omitempty"`
	ReviewerAgentID string                 `json:"reviewer_agent_id,omitempty"`
	DefaultMode     string                 `json:"default_mode,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt       time.Time              `json:"created_at,omitempty"`
	UpdatedAt       time.Time              `json:"updated_at,omitempty"`
}

func MetaKey(workspaceID string) string {
	return MetaKeyPrefix + workspaceID
}

func SessionIndexKey(workspaceID string) string {
	return SessionIndexKeyPrefix + workspaceID
}

func ChannelIndexKey(workspaceID string) string {
	return ChannelIndexKeyPrefix + workspaceID
}

func pathIndexKey(path string) string {
	return PathIndexKeyPrefix + filepath.Clean(path)
}

// SaveMeta persists workspace metadata and updates the optional path index.
func SaveMeta(storage middleware.Storage, meta Meta) error {
	if storage == nil {
		return fmt.Errorf("storage not available")
	}
	if strings.TrimSpace(meta.ID) == "" {
		return fmt.Errorf("workspace id is required")
	}
	now := time.Now().UTC()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now
	if meta.Name == "" {
		meta.Name = meta.ID
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to encode workspace %s: %w", meta.ID, err)
	}
	if err := storage.Set(MetaKey(meta.ID), data); err != nil {
		return fmt.Errorf("failed to store workspace %s: %w", meta.ID, err)
	}
	if strings.TrimSpace(meta.RootPath) != "" {
		if err := storage.Set(pathIndexKey(meta.RootPath), []byte(meta.ID)); err != nil {
			return fmt.Errorf("failed to store workspace path index for %s: %w", meta.ID, err)
		}
	}
	return nil
}

// LoadMeta returns workspace metadata for the given id.
func LoadMeta(storage middleware.Storage, workspaceID string) (Meta, bool, error) {
	if storage == nil {
		return Meta{}, false, fmt.Errorf("storage not available")
	}
	data, err := storage.Get(MetaKey(workspaceID))
	if err != nil {
		return Meta{}, false, fmt.Errorf("failed to read workspace %s: %w", workspaceID, err)
	}
	if len(data) == 0 {
		return Meta{}, false, nil
	}
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Meta{}, false, fmt.Errorf("failed to decode workspace %s: %w", workspaceID, err)
	}
	return meta, true, nil
}

// ResolveByPath resolves a workspace by its indexed root path.
func ResolveByPath(storage middleware.Storage, path string) (Meta, bool, error) {
	if storage == nil {
		return Meta{}, false, fmt.Errorf("storage not available")
	}
	if strings.TrimSpace(path) == "" {
		return Meta{}, false, nil
	}
	id, err := storage.Get(pathIndexKey(path))
	if err != nil {
		return Meta{}, false, fmt.Errorf("failed to read workspace path index: %w", err)
	}
	if len(id) == 0 {
		return Meta{}, false, nil
	}
	return LoadMeta(storage, string(id))
}

// ListMeta returns every stored workspace metadata entry sorted by id.
func ListMeta(storage middleware.Storage) ([]Meta, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage not available")
	}
	keys, err := storage.List(MetaKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspace metadata: %w", err)
	}
	metas := make([]Meta, 0, len(keys))
	for _, key := range keys {
		workspaceID := strings.TrimPrefix(key, MetaKeyPrefix)
		meta, found, err := LoadMeta(storage, workspaceID)
		if err != nil {
			return nil, err
		}
		if found {
			metas = append(metas, meta)
		}
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].ID < metas[j].ID })
	return metas, nil
}

// UpdateSessionIndex records a logical session as recently associated with the workspace.
func UpdateSessionIndex(storage middleware.Storage, workspaceID, sessionID string) error {
	return updateStringIndexWithLimit(storage, SessionIndexKey(workspaceID), sessionID, maxIndexedAssociations)
}

// RemoveSessionIndex removes a logical session from the workspace association index.
func RemoveSessionIndex(storage middleware.Storage, workspaceID, sessionID string) error {
	return removeStringIndexValue(storage, SessionIndexKey(workspaceID), sessionID)
}

// UpdateChannelIndex records a channel as recently associated with the workspace.
func UpdateChannelIndex(storage middleware.Storage, workspaceID, channelID string) error {
	return updateStringIndexWithLimit(storage, ChannelIndexKey(workspaceID), channelID, maxIndexedAssociations)
}

// LoadSessionIndex returns recent logical sessions associated with a workspace.
func LoadSessionIndex(storage middleware.Storage, workspaceID string) ([]string, error) {
	return loadStringIndex(storage, SessionIndexKey(workspaceID))
}

func loadStringIndex(storage middleware.Storage, key string) ([]string, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage not available")
	}
	data, err := storage.Get(key)
	if err != nil {
		return nil, fmt.Errorf("failed to read workspace index %s: %w", key, err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("failed to decode workspace index %s: %w", key, err)
	}
	return values, nil
}

func removeStringIndexValue(storage middleware.Storage, key, value string) error {
	values, err := loadStringIndex(storage, key)
	if err != nil {
		return err
	}
	if len(values) == 0 {
		return nil
	}
	next := make([]string, 0, len(values))
	for _, item := range values {
		if item != value {
			next = append(next, item)
		}
	}
	if len(next) == len(values) {
		return nil
	}
	data, err := json.Marshal(next)
	if err != nil {
		return fmt.Errorf("failed to encode workspace index %s: %w", key, err)
	}
	return storage.Set(key, data)
}
