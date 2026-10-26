package agentstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Get reads the immutable generation metadata for digest.
func (s *Store) Get(digest string) (Generation, error) {
	digest, err := validateDigest(digest)
	if err != nil {
		return Generation{}, err
	}
	data, err := os.ReadFile(s.generationPath(digest))
	if err != nil {
		if os.IsNotExist(err) {
			return Generation{}, fmt.Errorf("generation %s not found in agent store %s", digest, s.root)
		}
		return Generation{}, fmt.Errorf("read generation %s: %w", digest, err)
	}
	var generation Generation
	if err := json.Unmarshal(data, &generation); err != nil {
		return Generation{}, fmt.Errorf("decode generation %s: %w", digest, err)
	}
	if generation.Digest != digest {
		return Generation{}, fmt.Errorf("generation metadata mismatch: %s records digest %q", s.generationPath(digest), generation.Digest)
	}
	return generation, nil
}

// List returns every published generation sorted by digest. The manifest
// sidecar of each generation is not metadata and is skipped.
func (s *Store) List() ([]Generation, error) {
	entries, err := os.ReadDir(s.generationsDir())
	if err != nil {
		return nil, fmt.Errorf("list generations in %s: %w", s.generationsDir(), err)
	}
	generations := make([]Generation, 0, len(entries))
	for _, entry := range entries {
		digest, ok := metadataDigest(entry.Name())
		if !ok {
			continue
		}
		generation, err := s.Get(digest)
		if err != nil {
			return nil, err
		}
		generations = append(generations, generation)
	}
	sort.Slice(generations, func(i, j int) bool { return generations[i].Digest < generations[j].Digest })
	return generations, nil
}

func metadataDigest(name string) (string, bool) {
	digest, ok := strings.CutSuffix(name, ".json")
	if !ok {
		return "", false
	}
	if _, err := validateDigest(digest); err != nil {
		return "", false
	}
	return digest, true
}

// BlobDir is the absolute published tree directory for digest.
func (s *Store) BlobDir(digest string) (string, error) {
	digest, err := validateDigest(digest)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(s.blobsDir(), digest)
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("blob tree for generation %s is not published: %w", digest, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("blob tree for generation %s is not a directory", digest)
	}
	return dir, nil
}

// Verify recomputes the published tree manifest and compares it with the stored
// manifest and with the digest recorded in the generation metadata.
func (s *Store) Verify(digest string) error {
	digest, err := validateDigest(digest)
	if err != nil {
		return err
	}
	generation, err := s.Get(digest)
	if err != nil {
		return err
	}
	manifest, err := s.readManifest(digest)
	if err != nil {
		return err
	}
	manifestDigest, err := manifest.Digest()
	if err != nil {
		return err
	}
	if manifestDigest != generation.TreeManifestSHA256 {
		return fmt.Errorf("generation %s manifest digest mismatch: metadata records %s, stored manifest is %s", digest, generation.TreeManifestSHA256, manifestDigest)
	}
	tree, err := s.BlobDir(digest)
	if err != nil {
		return err
	}
	if err := VerifyTree(tree, manifest); err != nil {
		return fmt.Errorf("generation %s tree verification failed: %w", digest, err)
	}
	return nil
}

func (s *Store) readManifest(digest string) (Manifest, error) {
	data, err := os.ReadFile(s.manifestPath(digest))
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest for generation %s: %w", digest, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest for generation %s: %w", digest, err)
	}
	return manifest, nil
}
