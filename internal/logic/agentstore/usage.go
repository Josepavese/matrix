package agentstore

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// GenerationUsage is one generation measured on disk together with the number
// of the named homes that still reference it.
type GenerationUsage struct {
	Digest       string `json:"digest"`
	AgentID      string `json:"agent_id"`
	Version      string `json:"version"`
	Platform     string `json:"platform"`
	Bytes        int64  `json:"bytes"`
	ReferencedBy int    `json:"referenced_by"`
}

// Usage is the measured footprint of one store. Every generation is counted
// exactly once and classified by how many of the named homes reference it, so
// the three totals add up to TotalBytes by construction: exclusive (one home),
// shared (two or more) and reclaimable (none). The classification is the
// measurement a consumer needs before it leaves software out of a backup and
// the one garbage collection acts on when the last reference goes.
type Usage struct {
	Store            string            `json:"store"`
	Generations      []GenerationUsage `json:"generations"`
	TotalBytes       int64             `json:"total_bytes"`
	ExclusiveBytes   int64             `json:"exclusive_bytes"`
	SharedBytes      int64             `json:"shared_bytes"`
	ReclaimableBytes int64             `json:"reclaimable_bytes"`
	StagingBytes     int64             `json:"staging_bytes"`
}

// Measure reports what the store holds and who still needs it. fanout maps a
// digest to the number of named homes referencing it; a digest absent from it
// is referenced by none. The bytes are read from the published trees, not
// inferred from metadata: this is a measurement, so it makes no claim about
// what a backup would have cost.
func (s *Store) Measure(fanout map[string]int) (Usage, error) {
	generations, err := s.List()
	if err != nil {
		return Usage{}, err
	}
	usage := Usage{Store: s.root, Generations: make([]GenerationUsage, 0, len(generations))}
	for _, generation := range generations {
		bytes, err := s.blobBytes(generation.Digest)
		if err != nil {
			return Usage{}, err
		}
		referencedBy := fanout[generation.Digest]
		usage.addGeneration(generation, bytes, referencedBy)
	}
	staging, err := s.stagingBytes()
	if err != nil {
		return Usage{}, err
	}
	usage.StagingBytes = staging
	return usage, nil
}

// addGeneration records one measured generation and moves its bytes into the
// bucket its reference count selects.
func (u *Usage) addGeneration(generation Generation, bytes int64, referencedBy int) {
	u.Generations = append(u.Generations, GenerationUsage{
		Digest: generation.Digest, AgentID: generation.AgentID, Version: generation.Version,
		Platform: generation.Platform, Bytes: bytes, ReferencedBy: referencedBy,
	})
	u.TotalBytes += bytes
	switch {
	case referencedBy <= 0:
		u.ReclaimableBytes += bytes
	case referencedBy == 1:
		u.ExclusiveBytes += bytes
	default:
		u.SharedBytes += bytes
	}
}

// stagingBytes measures publish staging left under the store's tmp directory.
// It is work in flight, not published software, and it is reported apart for
// exactly that reason.
func (s *Store) stagingBytes() (int64, error) {
	entries, err := os.ReadDir(s.tmpDir())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("measure publish staging in %s: %w", s.tmpDir(), err)
	}
	var total int64
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "publish-") {
			continue
		}
		bytes, err := treeBytes(filepath.Join(s.tmpDir(), entry.Name()))
		if err != nil {
			return 0, err
		}
		total += bytes
	}
	return total, nil
}

// treeBytes sums the sizes of the regular files below tree.
func treeBytes(tree string) (int64, error) {
	var total int64
	err := filepath.WalkDir(tree, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("measure %s: %w", path, err)
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("measure tree %s: %w", tree, err)
	}
	return total, nil
}
