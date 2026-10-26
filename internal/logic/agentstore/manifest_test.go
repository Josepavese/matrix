package agentstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildManifestRecordsRegularFilesSorted(t *testing.T) {
	tree := t.TempDir()
	writeFile(t, filepath.Join(tree, "b.txt"), "bbb", 0o644)
	writeFile(t, filepath.Join(tree, "a", "run"), "#!/bin/sh\n", 0o755)
	writeFile(t, filepath.Join(tree, "a", "data.json"), "{}", 0o600)

	manifest, err := BuildManifest(tree)
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if manifest.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %d, want %d", manifest.SchemaVersion, SchemaVersion)
	}
	wantPaths := []string{"a/data.json", "a/run", "b.txt"}
	if len(manifest.Entries) != len(wantPaths) {
		t.Fatalf("entries = %d, want %d", len(manifest.Entries), len(wantPaths))
	}
	for i, want := range wantPaths {
		if manifest.Entries[i].Path != want {
			t.Fatalf("entry %d path = %q, want %q", i, manifest.Entries[i].Path, want)
		}
	}
	run := manifest.Entries[1]
	if run.Mode != 0o755 || run.Size != int64(len("#!/bin/sh\n")) {
		t.Fatalf("launcher entry = %+v", run)
	}
	if run.SHA256 != digestOfFile(t, filepath.Join(tree, "a", "run")) {
		t.Fatalf("launcher digest = %s", run.SHA256)
	}
}

func TestBuildManifestRejectsSymlinkEntry(t *testing.T) {
	if !supportsUnixPermissions() {
		t.Skip("symlinks are required")
	}
	tree := t.TempDir()
	writeFile(t, filepath.Join(tree, "real.txt"), "real", 0o644)
	if err := os.Symlink("real.txt", filepath.Join(tree, "link.txt")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	_, err := BuildManifest(tree)
	if err == nil {
		t.Fatal("expected a symlink in the tree to be rejected")
	}
	if !strings.Contains(err.Error(), "only regular files") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManifestDigestIsOrderIndependent(t *testing.T) {
	first := ManifestEntry{Path: "a.txt", Mode: 0o644, Size: 1, SHA256: hexDigest("b")}
	second := ManifestEntry{Path: "b.txt", Mode: 0o755, Size: 2, SHA256: hexDigest("c")}
	forward := Manifest{SchemaVersion: 1, Entries: []ManifestEntry{first, second}}
	reversed := Manifest{SchemaVersion: 1, Entries: []ManifestEntry{second, first}}
	forwardDigest, err := forward.Digest()
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	reversedDigest, err := reversed.Digest()
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if forwardDigest != reversedDigest {
		t.Fatalf("digest depends on entry order: %s != %s", forwardDigest, reversedDigest)
	}
	if len(forwardDigest) != 64 {
		t.Fatalf("digest is not 64 hex characters: %q", forwardDigest)
	}
	changed := Manifest{SchemaVersion: 1, Entries: []ManifestEntry{first, {Path: "b.txt", Mode: 0o755, Size: 2, SHA256: hexDigest("d")}}}
	changedDigest, err := changed.Digest()
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if changedDigest == forwardDigest {
		t.Fatal("digest did not change when entry content changed")
	}
}

func TestVerifyTreeDetectsTampering(t *testing.T) {
	tree := t.TempDir()
	writeFile(t, filepath.Join(tree, "a.txt"), "hello", 0o644)
	manifest, err := BuildManifest(tree)
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if err := VerifyTree(tree, manifest); err != nil {
		t.Fatalf("expected the untouched tree to verify: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tree, "a.txt"), []byte("hallo"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if err := VerifyTree(tree, manifest); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("tampered content not detected: %v", err)
	}

	writeFile(t, filepath.Join(tree, "a.txt"), "hello", 0o644)
	writeFile(t, filepath.Join(tree, "extra.txt"), "extra", 0o644)
	if err := VerifyTree(tree, manifest); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("extra file not detected: %v", err)
	}

	if err := os.Remove(filepath.Join(tree, "extra.txt")); err != nil {
		t.Fatalf("remove extra: %v", err)
	}
	if err := os.Remove(filepath.Join(tree, "a.txt")); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	if err := VerifyTree(tree, manifest); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing file not detected: %v", err)
	}
}

func TestVerifyTreeDetectsModeChange(t *testing.T) {
	tree := t.TempDir()
	writeFile(t, filepath.Join(tree, "a.txt"), "hello", 0o644)
	manifest, err := BuildManifest(tree)
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if err := os.Chmod(filepath.Join(tree, "a.txt"), 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := VerifyTree(tree, manifest); err == nil || !strings.Contains(err.Error(), "mode 0600") {
		t.Fatalf("mode change not detected: %v", err)
	}
}

func TestBuildManifestRejectsMissingTree(t *testing.T) {
	if _, err := BuildManifest(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected a missing tree to be rejected")
	}
	file := filepath.Join(t.TempDir(), "file")
	writeFile(t, file, "x", 0o644)
	if _, err := BuildManifest(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected a regular file tree to be rejected: %v", err)
	}
}
