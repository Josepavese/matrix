package agentstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Reference is one PAL home's pointer to a verified store generation.
type Reference struct {
	Store              string    `json:"store"`
	Digest             string    `json:"digest"`
	TreeManifestSHA256 string    `json:"tree_manifest_sha256"`
	Launcher           string    `json:"launcher"`
	Version            string    `json:"version"`
	Platform           string    `json:"platform"`
	InstalledAt        time.Time `json:"installed_at"`
}

// References is the per-home reference document.
type References struct {
	SchemaVersion int                  `json:"schema_version"`
	Agents        map[string]Reference `json:"agents"`
}

// ReadReferences reads the per-home reference document. A missing file is an
// empty document, not an error; malformed JSON is never silently reset.
func ReadReferences(palHome string) (References, error) {
	home := strings.TrimSpace(palHome)
	if home == "" {
		return References{}, fmt.Errorf("PAL home is required to read agent references")
	}
	path := ReferencePath(home)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return References{SchemaVersion: SchemaVersion, Agents: map[string]Reference{}}, nil
		}
		return References{}, fmt.Errorf("read agent references %s: %w", path, err)
	}
	var refs References
	if err := json.Unmarshal(data, &refs); err != nil {
		return References{}, fmt.Errorf("decode agent references %s: %w", path, err)
	}
	if refs.Agents == nil {
		refs.Agents = map[string]Reference{}
	}
	return refs, nil
}

// WriteReference merges ref for agentID into the home's reference document and
// replaces it atomically with 0600 permissions. Other agents are preserved.
func WriteReference(palHome, agentID string, ref Reference) error {
	home := strings.TrimSpace(palHome)
	if home == "" {
		return fmt.Errorf("PAL home is required to write an agent reference")
	}
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("agent id is required to write an agent reference")
	}
	refs, err := ReadReferences(home)
	if err != nil {
		return err
	}
	refs.SchemaVersion = SchemaVersion
	refs.Agents[agentID] = ref
	if err := os.MkdirAll(filepath.Dir(ReferencePath(home)), 0o700); err != nil {
		return fmt.Errorf("create agent reference directory: %w", err)
	}
	return writeJSONAtomic(ReferencePath(home), refs)
}

// RemoveReference deletes agentID's reference. Removing an entry that is not
// present is not an error.
func RemoveReference(palHome, agentID string) error {
	home := strings.TrimSpace(palHome)
	if home == "" {
		return fmt.Errorf("PAL home is required to remove an agent reference")
	}
	refs, err := ReadReferences(home)
	if err != nil {
		return err
	}
	if _, ok := refs.Agents[agentID]; !ok {
		return nil
	}
	delete(refs.Agents, agentID)
	return writeJSONAtomic(ReferencePath(home), refs)
}

// LoadRefSet unions the digests referenced by every given PAL home.
func LoadRefSet(homes []string) (RefSet, error) {
	fanout, err := LoadRefFanout(homes)
	if err != nil {
		return nil, err
	}
	refs := make(RefSet, len(fanout))
	for digest := range fanout {
		refs[digest] = struct{}{}
	}
	return refs, nil
}

// LoadRefFanout counts, per digest, how many of the given PAL homes reference
// it. A measurement needs the count rather than the union: a generation one
// home needs and a generation two homes share are different amounts of work to
// retire, and only the count tells them apart.
func LoadRefFanout(homes []string) (map[string]int, error) {
	fanout := make(map[string]int, len(homes))
	for _, home := range homes {
		references, err := ReadReferences(home)
		if err != nil {
			return nil, err
		}
		for _, ref := range references.Agents {
			if ref.Digest != "" {
				fanout[ref.Digest]++
			}
		}
	}
	return fanout, nil
}
