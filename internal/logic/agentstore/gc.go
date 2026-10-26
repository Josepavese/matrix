package agentstore

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pending is one published generation a garbage-collection pass can reclaim.
type Pending struct {
	Digest   string
	AgentID  string
	Version  string
	Platform string
	Bytes    int64
}

// RefSet is the set of digests some PAL home still references.
type RefSet map[string]struct{}

// Reclaimable describes the unreferenced generations and their on-disk bytes
// without touching anything. It is safe to call on a read-only consumer store.
func (s *Store) Reclaimable(refs RefSet) ([]Pending, error) {
	generations, err := s.List()
	if err != nil {
		return nil, err
	}
	pending := make([]Pending, 0, len(generations))
	for _, generation := range generations {
		if _, referenced := refs[generation.Digest]; referenced {
			continue
		}
		bytes, err := s.blobBytes(generation.Digest)
		if err != nil {
			return nil, err
		}
		pending = append(pending, Pending{
			Digest:   generation.Digest,
			AgentID:  generation.AgentID,
			Version:  generation.Version,
			Platform: generation.Platform,
			Bytes:    bytes,
		})
	}
	return pending, nil
}

// GC removes every generation absent from refs, plus publish staging left by
// interrupted publications older than one hour, and returns what was deleted and
// the bytes freed. Referenced generations are never touched.
func (s *Store) GC(refs RefSet) ([]Pending, int64, error) {
	if s.readOnly {
		return nil, 0, errReadOnlyStore
	}
	pending, err := s.Reclaimable(refs)
	if err != nil {
		return nil, 0, err
	}
	var freed int64
	for _, item := range pending {
		if err := s.deleteGeneration(item.Digest); err != nil {
			return pending, freed, err
		}
		freed += item.Bytes
	}
	if err := s.cleanStaging(time.Hour); err != nil {
		return pending, freed, err
	}
	return pending, freed, nil
}

func (s *Store) deleteGeneration(digest string) error {
	blob := filepath.Join(s.blobsDir(), digest)
	if err := makeTreeWritable(blob); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("prepare blob tree %s for removal: %w", digest, err)
	}
	if err := os.RemoveAll(blob); err != nil {
		return fmt.Errorf("remove blob tree %s: %w", digest, err)
	}
	for _, metadata := range []string{s.manifestPath(digest), s.generationPath(digest)} {
		if err := os.Remove(metadata); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove agent store metadata %s: %w", metadata, err)
		}
	}
	return nil
}

// makeTreeWritable restores write permission on a sealed tree so it can be
// removed. Directories also need the execute bit to stay traversable.
func makeTreeWritable(tree string) error {
	return filepath.WalkDir(tree, func(entryPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(entryPath, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.Chmod(entryPath, info.Mode().Perm()|0o200)
	})
}

func (s *Store) cleanStaging(maxAge time.Duration) error {
	entries, err := os.ReadDir(s.tmpDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("list publish staging in %s: %w", s.tmpDir(), err)
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "publish-") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		staging := filepath.Join(s.tmpDir(), entry.Name())
		_ = makeTreeWritable(staging)
		if err := os.RemoveAll(staging); err != nil {
			return fmt.Errorf("remove abandoned publish staging %s: %w", staging, err)
		}
	}
	return nil
}

func (s *Store) blobBytes(digest string) (int64, error) {
	var total int64
	err := filepath.WalkDir(filepath.Join(s.blobsDir(), digest), func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return 0, fmt.Errorf("measure blob tree %s: %w", digest, err)
	}
	return total, nil
}
