// Package semanticfs exposes deliberately selected Matrix state as a read-only
// fs.FS. No vault/config key namespace or provider credential is projected.
package semanticfs

import (
	"encoding/hex"
	"fmt"
	"io/fs"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Josepavese/matrix/internal/middleware"
)

type AgentView struct {
	Source    string `json:"source"`
	ID        string `json:"id"`
	Active    *bool  `json:"active,omitempty"`
	Kind      string `json:"kind"`
	Transport string `json:"transport"`
}

type Source struct {
	Storage          middleware.Storage
	Agents           func() ([]AgentView, error)
	Capacity         middleware.Capacity
	IncludeSummaries bool
	MaxEntries       int
}

type FS struct {
	source Source
	epoch  time.Time
}

func New(source Source) *FS {
	if source.MaxEntries == 0 {
		source.MaxEntries = 4096
	}
	return &FS{source: source, epoch: time.Now().UTC()}
}

// DirectoryName is reversible and case-preserving across case-insensitive
// filesystems. The fixed prefix avoids Windows reserved device names.
func DirectoryName(id string) (string, error) {
	if id == "" || len(id) > 125 || !utf8.ValidString(id) {
		return "", fmt.Errorf("semantic directory ID must contain 1 to 125 UTF-8 bytes")
	}
	return "id-" + hex.EncodeToString([]byte(id)), nil
}

func decodeID(name string) (string, error) {
	if !strings.HasPrefix(name, "id-") {
		return "", fs.ErrNotExist
	}
	b, err := hex.DecodeString(strings.TrimPrefix(name, "id-"))
	if err != nil {
		return "", fs.ErrNotExist
	}
	id := string(b)
	canonical, err := DirectoryName(id)
	if err != nil || canonical != name {
		return "", fs.ErrNotExist
	}
	return id, nil
}
