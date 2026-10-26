// Package agentstore implements an immutable, content-addressed store of agent
// artifacts together with the per-home references that point at a verified
// generation.
//
// The store lives outside every PAL home so a Hub backup does not pay for the
// same bytes once per worker. A generation is published once, keyed by the
// sha256 of the archive Matrix already verified, extracted into a read-only
// tree and described by a deterministic manifest. Consumers open the store
// read-only: OpenConsumer fails closed unless the store root, blobs,
// generations and blobs/sha256 directories are all read-only for the runtime
// user, because a runtime that can write them can swap the verified bytes after
// installation. Sealing the layout is the deployment's job ("publish from a
// different account or mount the store read-only"); a publisher-created layout
// is 0700 by design and is therefore refused to consumers until sealed.
//
// Read-only here means "no write bits": every directory is sealed 0555 and every
// file loses its write bits, so executables stay executable (0555) and data
// files become 0444. A store that could not run what it stores would only move
// the problem. Archive extraction is delegated to the injected
// middleware.Archive provider; the store keeps its own digest gate and its own
// post-extraction safety walk on top of it.
package agentstore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// EnvName is the deployment contract naming the absolute store path.
	EnvName = "MATRIX_AGENT_STORE"
	// SchemaVersion is the metadata schema this package reads and writes.
	SchemaVersion = 1
)

const (
	blobsDirName       = "blobs"
	generationsDirName = "generations"
	tmpDirName         = "tmp"
	sha256DirName      = "sha256"
	referenceFileName  = "agent_references.json"
)

// ValidateRoot normalizes an absolute store root and refuses a root equal to or
// inside the PAL home it is meant to serve; a store inside the home would be
// backed up with the home and defeat the deduplication.
func ValidateRoot(root, palHome string) (string, error) {
	resolved, err := resolveStorePath(root)
	if err != nil {
		return "", err
	}
	home := strings.TrimSpace(palHome)
	if home == "" {
		return resolved, nil
	}
	resolvedHome, err := resolveStorePath(home)
	if err != nil {
		return "", fmt.Errorf("resolve PAL home %q: %w", palHome, err)
	}
	if isWithin(resolved, resolvedHome) {
		return "", fmt.Errorf("agent store %s must live outside the PAL home %s", resolved, resolvedHome)
	}
	return resolved, nil
}

// resolveStorePath makes path absolute, resolves every existing symlink and
// accepts a path that does not exist yet as long as its nearest existing
// ancestor is a directory the layout can be created under.
func resolveStorePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || !filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("agent store path must be a non-empty absolute path: %q", path)
	}
	clean := filepath.Clean(trimmed)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve agent store path %s: %w", clean, err)
		}
		parent, err := resolveStorePath(filepath.Dir(clean))
		if err != nil {
			return "", err
		}
		return filepath.Join(parent, filepath.Base(clean)), nil
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect agent store path %s: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("agent store path is not a directory: %s", resolved)
	}
	return resolved, nil
}

// isWithin reports whether path is base itself or lives under base.
func isWithin(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// ReferencePath is where a PAL home records the generations it references.
func ReferencePath(palHome string) string {
	return filepath.Join(palHome, "configs", referenceFileName)
}

// validateDigest accepts exactly 64 lowercase hex characters. The sha256:
// prefix is refused because the bare hex value is the identity used by the
// generation metadata and by every consumer.
func validateDigest(digest string) (string, error) {
	if len(digest) == sha256.Size*2 && strings.ToLower(digest) == digest {
		if _, err := hex.DecodeString(digest); err == nil {
			return digest, nil
		}
	}
	if strings.HasPrefix(strings.ToLower(digest), "sha256:") {
		return "", fmt.Errorf("invalid artifact digest %q: use the bare 64-character hex digest without the sha256: prefix", digest)
	}
	return "", fmt.Errorf("invalid artifact digest %q: expected 64 lowercase hex characters", digest)
}
