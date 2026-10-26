package agentstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReclaimableDryRunKeepsGenerations(t *testing.T) {
	store := newTestPublisher(t)
	_, firstDigest, _ := publishTarGz(t, store, launcherEntries())
	secondEntries := launcherEntries()
	secondEntries[1].body = "second artifact payload"
	_, secondDigest, _ := publishTarGz(t, store, secondEntries)

	pending, err := store.Reclaimable(RefSet{})
	if err != nil {
		t.Fatalf("reclaimable: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}
	seen := map[string]bool{}
	for _, item := range pending {
		if item.Bytes <= 0 {
			t.Fatalf("pending %s reports %d bytes", item.Digest, item.Bytes)
		}
		seen[item.Digest] = true
	}
	if !seen[firstDigest] || !seen[secondDigest] {
		t.Fatalf("pending misses a generation: %+v", pending)
	}
	list, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("dry run removed generations: %d left", len(list))
	}
}

func TestGCDeletesOnlyUnreferenced(t *testing.T) {
	store := newTestPublisher(t)
	_, keepDigest, _ := publishTarGz(t, store, launcherEntries())
	secondEntries := launcherEntries()
	secondEntries[1].body = "second artifact payload"
	_, dropDigest, _ := publishTarGz(t, store, secondEntries)

	refs := RefSet{keepDigest: struct{}{}}
	removed, freed, err := store.GC(refs)
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if len(removed) != 1 || removed[0].Digest != dropDigest {
		t.Fatalf("removed = %+v, want only %s", removed, dropDigest)
	}
	if freed != removed[0].Bytes || freed <= 0 {
		t.Fatalf("freed = %d, want %d (> 0)", freed, removed[0].Bytes)
	}
	if _, err := store.BlobDir(dropDigest); err == nil {
		t.Fatal("unreferenced blob tree survived GC")
	}
	for _, metadata := range []string{store.generationPath(dropDigest), store.manifestPath(dropDigest)} {
		if _, err := os.Stat(metadata); !os.IsNotExist(err) {
			t.Fatalf("metadata %s survived GC: %v", metadata, err)
		}
	}
	if err := store.Verify(keepDigest); err != nil {
		t.Fatalf("referenced generation damaged by GC: %v", err)
	}
	if _, err := store.BlobDir(keepDigest); err != nil {
		t.Fatalf("referenced blob tree removed: %v", err)
	}
	remaining, err := store.Reclaimable(refs)
	if err != nil {
		t.Fatalf("reclaimable after gc: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("unreferenced generations remain: %+v", remaining)
	}
}

func TestGCWithoutRefsDeletesAll(t *testing.T) {
	store := newTestPublisher(t)
	_, firstDigest, _ := publishTarGz(t, store, launcherEntries())
	secondEntries := launcherEntries()
	secondEntries[1].body = "second artifact payload"
	_, secondDigest, _ := publishTarGz(t, store, secondEntries)

	removed, freed, err := store.GC(nil)
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if len(removed) != 2 || freed <= 0 {
		t.Fatalf("removed = %d, freed = %d", len(removed), freed)
	}
	for _, digest := range []string{firstDigest, secondDigest} {
		if _, err := store.BlobDir(digest); err == nil {
			t.Fatalf("blob tree %s survived GC", digest)
		}
	}
	list, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("generations left after GC: %+v", list)
	}
}

func TestGCOnConsumerIsRefused(t *testing.T) {
	store := newTestPublisher(t)
	publishTarGz(t, store, launcherEntries())
	sealStoreForConsumer(t, store)
	consumer, err := OpenConsumer(store.Root(), "")
	if err != nil {
		t.Fatalf("open consumer: %v", err)
	}
	if _, _, err := consumer.GC(RefSet{}); !errors.Is(err, errReadOnlyStore) {
		t.Fatalf("consumer GC = %v, want errReadOnlyStore", err)
	}
}

func TestGCRemovesAbandonedStagingOnly(t *testing.T) {
	store := newTestPublisher(t)
	abandoned := filepath.Join(store.tmpDir(), "publish-"+hexDigest("a")+"-abandoned")
	writeFile(t, filepath.Join(abandoned, "partial", "file"), "partial", 0o600)
	old := oldTimestamp()
	if err := os.Chtimes(abandoned, old, old); err != nil {
		t.Fatalf("age staging dir: %v", err)
	}
	fresh := filepath.Join(store.tmpDir(), "publish-"+hexDigest("b")+"-fresh")
	if err := os.MkdirAll(fresh, 0o700); err != nil {
		t.Fatalf("create fresh staging: %v", err)
	}

	removed, freed, err := store.GC(RefSet{})
	if err != nil {
		t.Fatalf("gc: %v", err)
	}
	if len(removed) != 0 || freed != 0 {
		t.Fatalf("empty store GC = %+v, %d", removed, freed)
	}
	if _, err := os.Stat(abandoned); !os.IsNotExist(err) {
		t.Fatalf("abandoned staging survived GC: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh staging was removed: %v", err)
	}
	if err := os.RemoveAll(fresh); err != nil {
		t.Fatalf("remove fresh staging: %v", err)
	}
}
