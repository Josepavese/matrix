// Package agentcfg manages agent configuration metadata stored in the vault.
package agentcfg

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// MetaKeyPrefix is the vault key prefix for agent metadata entries.
const MetaKeyPrefix = "agent.meta."

// ArtifactVerificationStatus is the outcome of the integrity check Matrix runs
// on the artifact it downloads from the ACP registry.
type ArtifactVerificationStatus string

const (
	// ArtifactVerified means the sha256 of the downloaded artifact matched the
	// digest published by the registry index for this platform.
	ArtifactVerified ArtifactVerificationStatus = "verified"
	// ArtifactDigestNotPublished means the index published the distribution
	// without a sha256 for this platform or agent: the install proceeds, but
	// nothing was verified.
	ArtifactDigestNotPublished ArtifactVerificationStatus = "digest_not_published"
	// ArtifactNotApplicable means the distribution type downloads no artifact
	// (npx/uvx): there is no byte range to hash.
	ArtifactNotApplicable ArtifactVerificationStatus = "not_applicable"
)

// ArtifactVerification is the integrity evidence Matrix records at install time
// for the artifact it downloaded. It is the single answer to "was the digest
// verified?", read back by `matrix doctor`, `matrix agent doctor` and
// `matrix agent info --source=local`.
//
// Verified is true only when the computed digest matched the published one: an
// absent publication and an inapplicable distribution are both false, because
// "not published" and "verified" are different facts.
type ArtifactVerification struct {
	Verified   bool                       `json:"verified"`
	Status     ArtifactVerificationStatus `json:"status"`
	Platform   string                     `json:"platform,omitempty"`
	Artifact   string                     `json:"artifact,omitempty"`
	Expected   string                     `json:"expected_sha256,omitempty"`
	Actual     string                     `json:"actual_sha256,omitempty"`
	VerifiedAt time.Time                  `json:"verified_at"`
}

// Describe renders the outcome as one human-readable line. "not published" and
// "not applicable" never read as "verified".
func (v *ArtifactVerification) Describe() string {
	if v == nil {
		return "not recorded"
	}
	switch v.Status {
	case ArtifactVerified:
		return fmt.Sprintf("sha256 verified against the registry index for %s", v.Platform)
	case ArtifactDigestNotPublished:
		return fmt.Sprintf("not verified: the registry index publishes no sha256 for %s", v.Platform)
	case ArtifactNotApplicable:
		return "not applicable: this distribution downloads no artifact"
	default:
		return fmt.Sprintf("not verified (%s)", v.Status)
	}
}

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
	// ArtifactVerification is absent for agents that were not installed by the
	// installer (seeded from config, registered A2A endpoints).
	ArtifactVerification *ArtifactVerification `json:"artifact_verification,omitempty"`
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
