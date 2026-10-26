package agentstore

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// Generation is the immutable metadata of one published artifact tree.
type Generation struct {
	SchemaVersion      int       `json:"schema_version"`
	AgentID            string    `json:"agent_id"`
	Version            string    `json:"version"`
	Platform           string    `json:"platform"`
	Artifact           string    `json:"artifact"`
	Digest             string    `json:"digest"`
	Launcher           string    `json:"launcher"`
	Args               []string  `json:"args,omitempty"`
	License            string    `json:"license,omitempty"`
	Repository         string    `json:"repository,omitempty"`
	TreeManifestSHA256 string    `json:"tree_manifest_sha256"`
	PublishedAt        time.Time `json:"published_at"`
}

// PublishArchive verifies the archive digest Matrix already trusts, extracts
// the tree through the injected archive provider, seals it read-only and
// publishes it under blobs/sha256/<digest> together with its metadata. The
// extraction provider is injected so the store reuses the deployment's
// platform archive handling instead of a second implementation. Publishing an
// existing generation is idempotent when the immutable fields match exactly;
// otherwise it fails with an immutability error.
func (s *Store) PublishArchive(arch middleware.Archive, archivePath, digest string, g Generation) (Generation, error) {
	if s.readOnly {
		return Generation{}, errReadOnlyStore
	}
	if arch == nil {
		return Generation{}, fmt.Errorf("archive provider is required to publish a generation")
	}
	digest, err := validateDigest(digest)
	if err != nil {
		return Generation{}, err
	}
	if err := verifyArchiveDigest(archivePath, digest); err != nil {
		return Generation{}, err
	}
	g.Digest = digest
	existing, found, err := s.lookupGeneration(digest)
	if err != nil {
		return Generation{}, err
	}
	if found {
		return existing, requireSameGeneration(existing, g)
	}
	return s.publishStaged(arch, archivePath, digest, g)
}

func (s *Store) publishStaged(arch middleware.Archive, archivePath, digest string, g Generation) (Generation, error) {
	staging, err := os.MkdirTemp(s.tmpDir(), "publish-"+digest+"-")
	if err != nil {
		return Generation{}, fmt.Errorf("create publish staging in %s: %w", s.tmpDir(), err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err := arch.Extract(archivePath, staging); err != nil {
		return Generation{}, fmt.Errorf("extract artifact archive: %w", err)
	}
	manifest, err := stageGeneration(staging, digest, &g)
	if err != nil {
		return Generation{}, err
	}
	if err := s.commitTree(staging, digest); err != nil {
		return Generation{}, err
	}
	if err := writeJSONAtomic(s.manifestPath(digest), manifest); err != nil {
		return Generation{}, err
	}
	if err := writeJSONAtomic(s.generationPath(digest), g); err != nil {
		return Generation{}, err
	}
	return g, nil
}

// stageGeneration validates the extracted tree, seals it read-only, then records
// the generation fields that must be derived from the tree itself. The launcher
// string is preserved verbatim: callers record the value the registry published.
func stageGeneration(staging, digest string, g *Generation) (Manifest, error) {
	if err := rejectUnsupportedEntries(staging); err != nil {
		return Manifest{}, err
	}
	if err := validateLauncher(staging, g.Launcher); err != nil {
		return Manifest{}, err
	}
	if err := sealTreeContents(staging); err != nil {
		return Manifest{}, err
	}
	manifest, err := BuildManifest(staging)
	if err != nil {
		return Manifest{}, err
	}
	manifestDigest, err := manifest.Digest()
	if err != nil {
		return Manifest{}, err
	}
	g.SchemaVersion = SchemaVersion
	g.Digest = digest
	g.TreeManifestSHA256 = manifestDigest
	g.PublishedAt = time.Now().UTC()
	return manifest, nil
}

func (s *Store) lookupGeneration(digest string) (Generation, bool, error) {
	data, err := os.ReadFile(s.generationPath(digest))
	if os.IsNotExist(err) {
		return Generation{}, false, nil
	}
	if err != nil {
		return Generation{}, false, fmt.Errorf("read generation %s: %w", digest, err)
	}
	var existing Generation
	if err := json.Unmarshal(data, &existing); err != nil {
		return Generation{}, false, fmt.Errorf("decode generation %s: %w", digest, err)
	}
	if err := s.Verify(digest); err != nil {
		return Generation{}, false, err
	}
	return existing, true, nil
}

func requireSameGeneration(existing, incoming Generation) error {
	var changes []string
	for _, field := range [][3]string{
		{"agent_id", existing.AgentID, incoming.AgentID},
		{"version", existing.Version, incoming.Version},
		{"platform", existing.Platform, incoming.Platform},
		{"artifact", existing.Artifact, incoming.Artifact},
		{"digest", existing.Digest, incoming.Digest},
		{"launcher", existing.Launcher, incoming.Launcher},
	} {
		if field[1] != field[2] {
			changes = append(changes, fmt.Sprintf("%s (%q -> %q)", field[0], field[1], field[2]))
		}
	}
	if len(changes) == 0 {
		return nil
	}
	return fmt.Errorf("generation %s is immutable: republishing it would change %s", existing.Digest, strings.Join(changes, ", "))
}

func verifyArchiveDigest(archivePath, digest string) error {
	actual, err := hashFile(archivePath)
	if err != nil {
		return fmt.Errorf("hash artifact archive: %w", err)
	}
	if actual != digest {
		return fmt.Errorf("artifact archive digest mismatch: expected %s, computed %s", digest, actual)
	}
	return nil
}

// rejectUnsupportedEntries refuses symlinks, devices, sockets and hardlinks in
// the extracted tree with an explicit Lstat walk, so a hostile archive cannot
// smuggle a path that escapes the generation.
func rejectUnsupportedEntries(tree string) error {
	return filepath.WalkDir(tree, func(entryPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect extracted entry %s: %w", entryPath, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact tree contains %q (%s); symlinks and special files are not allowed", entryPath, info.Mode())
		}
		return nil
	})
}

// validateLauncher requires a relative path that stays inside the tree and
// names a regular file. The launcher string itself is preserved verbatim.
func validateLauncher(tree, launcher string) error {
	launcher = strings.TrimSpace(launcher)
	cleaned := path.Clean(filepath.ToSlash(launcher))
	if launcher == "" || filepath.IsAbs(launcher) || strings.HasPrefix(launcher, "/") {
		return fmt.Errorf("generation launcher must be a relative path inside the artifact tree: %q", launcher)
	}
	if cleaned == "." || cleaned == ".." || path.IsAbs(cleaned) || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("generation launcher %q escapes the artifact tree", launcher)
	}
	info, err := os.Lstat(filepath.Join(tree, filepath.FromSlash(cleaned)))
	if err != nil {
		return fmt.Errorf("generation launcher %q not found in artifact: %w", launcher, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("generation launcher %q is not a regular file", launcher)
	}
	return nil
}

// sealTreeContents removes every write bit below tree and seals directories
// 0555; exec bits are preserved so the launcher stays executable from the store.
// The tree root is deliberately left writable because Linux refuses to rename a
// directory the process cannot write to; the root is sealed once it is in place.
func sealTreeContents(tree string) error {
	return filepath.WalkDir(tree, func(entryPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entryPath == tree {
			return nil
		}
		if entry.IsDir() {
			if err := os.Chmod(entryPath, 0o555); err != nil {
				return fmt.Errorf("seal directory %s: %w", entryPath, err)
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect extracted entry %s: %w", entryPath, err)
		}
		if err := os.Chmod(entryPath, info.Mode().Perm()&^0o222); err != nil {
			return fmt.Errorf("seal file %s: %w", entryPath, err)
		}
		return nil
	})
}

func (s *Store) commitTree(staging, digest string) error {
	target := filepath.Join(s.blobsDir(), digest)
	if err := os.Rename(staging, target); err != nil {
		if _, statErr := os.Lstat(target); statErr != nil {
			return fmt.Errorf("commit generation %s: %w", digest, err)
		}
	}
	if err := os.Chmod(target, 0o555); err != nil {
		return fmt.Errorf("seal published tree %s: %w", target, err)
	}
	return nil
}

func writeJSONAtomic(filePath string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(filePath), err)
	}
	dir := filepath.Dir(filePath)
	temp, err := os.CreateTemp(dir, ".agentstore-*")
	if err != nil {
		return fmt.Errorf("create temporary metadata file in %s: %w", dir, err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
	_, writeErr := temp.Write(append(data, '\n'))
	syncErr := temp.Sync()
	closeErr := temp.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return fmt.Errorf("write %s: write=%v sync=%v close=%v", filepath.Base(filePath), writeErr, syncErr, closeErr)
	}
	if err := os.Rename(tempName, filePath); err != nil {
		return fmt.Errorf("replace %s: %w", filePath, err)
	}
	return nil
}
