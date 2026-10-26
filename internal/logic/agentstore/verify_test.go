package agentstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetMissingGeneration(t *testing.T) {
	store := newTestPublisher(t)
	if _, err := store.Get(hexDigest("a")); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a missing generation error: %v", err)
	}
}

func TestGetRejectsMalformedDigest(t *testing.T) {
	store := newTestPublisher(t)
	for _, digest := range []string{"", "sha256:" + hexDigest("a"), strings.ToUpper(hexDigest("a")), "abc"} {
		if _, err := store.Get(digest); err == nil {
			t.Fatalf("expected digest %q to be rejected", digest)
		}
	}
}

func TestListSortedAndSkipsManifestSidecars(t *testing.T) {
	store := newTestPublisher(t)
	_, firstDigest, _ := publishTarGz(t, store, launcherEntries())
	secondEntries := launcherEntries()
	secondEntries[1].body = "a different second artifact"
	_, secondDigest, _ := publishTarGz(t, store, secondEntries)

	list, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("generations = %d, want 2", len(list))
	}
	if list[0].Digest > list[1].Digest {
		t.Fatalf("list is not sorted by digest: %s, %s", list[0].Digest, list[1].Digest)
	}
	seen := map[string]bool{list[0].Digest: true, list[1].Digest: true}
	for _, digest := range []string{firstDigest, secondDigest} {
		if !seen[digest] {
			t.Fatalf("generation %s missing from list", digest)
		}
		if _, err := os.Stat(store.manifestPath(digest)); err != nil {
			t.Fatalf("manifest sidecar for %s missing: %v", digest, err)
		}
	}
}

func TestBlobDirRejectsUnknownGeneration(t *testing.T) {
	store := newTestPublisher(t)
	if _, err := store.BlobDir(hexDigest("f")); err == nil {
		t.Fatal("expected an unknown generation to be rejected")
	}
}

func TestVerifyDetectsTamperedByte(t *testing.T) {
	store := newTestPublisher(t)
	_, digest, _ := publishTarGz(t, store, launcherEntries())
	blobDir, err := store.BlobDir(digest)
	if err != nil {
		t.Fatalf("blob dir: %v", err)
	}
	unlockForTest(t, blobDir)
	launcher := filepath.Join(blobDir, "opencode")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\necho tampered\n"), 0o755); err != nil {
		t.Fatalf("tamper launcher: %v", err)
	}
	relockForTest(t, blobDir)
	err = store.Verify(digest)
	if err == nil || !strings.Contains(err.Error(), "tree verification failed") {
		t.Fatalf("tampered tree not detected: %v", err)
	}
}

func TestVerifyDetectsMissingFile(t *testing.T) {
	store := newTestPublisher(t)
	_, digest, _ := publishTarGz(t, store, launcherEntries())
	blobDir, err := store.BlobDir(digest)
	if err != nil {
		t.Fatalf("blob dir: %v", err)
	}
	unlockForTest(t, blobDir)
	if err := os.Remove(filepath.Join(blobDir, "lib", "data.txt")); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	relockForTest(t, blobDir)
	err = store.Verify(digest)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing file not detected: %v", err)
	}
}

func TestVerifyDetectsExtraFile(t *testing.T) {
	store := newTestPublisher(t)
	_, digest, _ := publishTarGz(t, store, launcherEntries())
	blobDir, err := store.BlobDir(digest)
	if err != nil {
		t.Fatalf("blob dir: %v", err)
	}
	unlockForTest(t, blobDir)
	extra := filepath.Join(blobDir, "smuggled.sh")
	if err := os.WriteFile(extra, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write extra file: %v", err)
	}
	relockForTest(t, blobDir)
	err = store.Verify(digest)
	if err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("extra file not detected: %v", err)
	}
}

func unlockForTest(t *testing.T, tree string) {
	t.Helper()
	if !supportsUnixPermissions() {
		return
	}
	if err := makeTreeWritable(tree); err != nil {
		t.Fatalf("unlock tree %s: %v", tree, err)
	}
}

func relockForTest(t *testing.T, tree string) {
	t.Helper()
	if !supportsUnixPermissions() {
		return
	}
	if err := sealTreeContents(tree); err != nil {
		t.Fatalf("relock tree %s: %v", tree, err)
	}
	if err := os.Chmod(tree, 0o555); err != nil {
		t.Fatalf("relock tree root %s: %v", tree, err)
	}
}
