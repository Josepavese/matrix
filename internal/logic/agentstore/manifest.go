package agentstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// ManifestEntry describes one regular file inside a published generation tree.
type ManifestEntry struct {
	Path   string `json:"path"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest is the deterministic inventory of a published generation tree.
type Manifest struct {
	SchemaVersion int             `json:"schema_version"`
	Entries       []ManifestEntry `json:"entries"`
}

// BuildManifest walks tree and records every regular file with its permission
// bits, size and content digest, sorted by path. Symlinks and every other
// non-regular entry are refused: a published generation is a plain tree.
func BuildManifest(tree string) (Manifest, error) {
	absolute, err := filepath.Abs(tree)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve artifact tree %q: %w", tree, err)
	}
	entries := []ManifestEntry{}
	err = filepath.WalkDir(absolute, func(entryPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entryPath == absolute && !entry.IsDir() {
			return fmt.Errorf("artifact tree is not a directory: %s", absolute)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect %s: %w", entryPath, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact tree contains %q (%s); only regular files are allowed", entryPath, info.Mode())
		}
		relative, err := filepath.Rel(absolute, entryPath)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", entryPath, err)
		}
		sum, err := hashFile(entryPath)
		if err != nil {
			return err
		}
		entries = append(entries, ManifestEntry{filepath.ToSlash(relative), uint32(info.Mode().Perm()), info.Size(), sum})
		return nil
	})
	if err != nil {
		return Manifest{}, fmt.Errorf("build manifest for %s: %w", absolute, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return Manifest{SchemaVersion: SchemaVersion, Entries: entries}, nil
}

// hashFile returns the lowercase hex sha256 of a file's bytes.
func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Digest is the sha256 over the canonical JSON of the entries sorted by path.
// The sort makes the digest independent of the order the caller supplied.
func (m Manifest) Digest() (string, error) {
	entries := make([]ManifestEntry, len(m.Entries))
	copy(entries, m.Entries)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	canonical, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("encode manifest entries: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyTree rebuilds the manifest of tree and compares the exact file set and
// each entry's permission bits, size and content digest against m.
func VerifyTree(tree string, m Manifest) error {
	actual, err := BuildManifest(tree)
	if err != nil {
		return err
	}
	expected := make(map[string]ManifestEntry, len(m.Entries))
	for _, entry := range m.Entries {
		expected[entry.Path] = entry
	}
	for _, entry := range actual.Entries {
		want, ok := expected[entry.Path]
		if !ok {
			return fmt.Errorf("artifact tree contains unexpected file %q", entry.Path)
		}
		if want.Mode != entry.Mode || want.Size != entry.Size || want.SHA256 != entry.SHA256 {
			return fmt.Errorf("manifest entry %q mismatch: expected mode %04o size %d sha256 %s, found mode %04o size %d sha256 %s",
				entry.Path, want.Mode, want.Size, want.SHA256, entry.Mode, entry.Size, entry.SHA256)
		}
		delete(expected, entry.Path)
	}
	if len(expected) == 0 {
		return nil
	}
	paths := make([]string, 0, len(expected))
	for path := range expected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return fmt.Errorf("artifact tree is missing %d manifest file(s), first %q", len(paths), paths[0])
}
