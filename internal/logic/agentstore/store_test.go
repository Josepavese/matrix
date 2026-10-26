package agentstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenPublisherCreatesPrivateLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	store, err := OpenPublisher(root, "")
	if err != nil {
		t.Fatalf("open publisher: %v", err)
	}
	dirs := append(store.layoutDirs(), store.tmpDir())
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", dir)
		}
		if supportsUnixPermissions() && info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %04o, want 0700", dir, info.Mode().Perm())
		}
	}
	if store.Root() == "" || !filepath.IsAbs(store.Root()) {
		t.Fatalf("Root() = %q, want an absolute path", store.Root())
	}
}

func TestOpenPublisherRejectsStoreInsidePalHome(t *testing.T) {
	home := t.TempDir()
	if _, err := OpenPublisher(filepath.Join(home, "store"), home); err == nil {
		t.Fatal("expected a store inside the PAL home to be rejected")
	}
}

func TestOpenConsumerRejectsPublisherLayout(t *testing.T) {
	store := newTestPublisher(t)
	_, err := OpenConsumer(store.Root(), "")
	if err == nil {
		t.Fatal("expected a writable store to be refused")
	}
	for _, fragment := range []string{"writable by the current user", "publish from a different account or mount the store read-only"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not name %q", err, fragment)
		}
	}
}

func TestOpenConsumerRequiresLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "empty-store")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("create root: %v", err)
	}
	if _, err := OpenConsumer(root, ""); err == nil || !strings.Contains(err.Error(), "layout is not usable") {
		t.Fatalf("expected a missing layout to be refused: %v", err)
	}
}

func TestOpenConsumerAcceptsSealedStore(t *testing.T) {
	store := newTestPublisher(t)
	_, digest, _ := publishTarGz(t, store, launcherEntries())
	sealStoreForConsumer(t, store)

	consumer, err := OpenConsumer(store.Root(), "")
	if err != nil {
		t.Fatalf("open sealed store as consumer: %v", err)
	}
	if _, err := consumer.Get(digest); err != nil {
		t.Fatalf("consumer Get: %v", err)
	}
	if err := consumer.Verify(digest); err != nil {
		t.Fatalf("consumer Verify: %v", err)
	}
	if _, err := consumer.PublishArchive(testArchiver(), "ignored.tar.gz", digest, sampleGeneration()); !errors.Is(err, errReadOnlyStore) {
		t.Fatalf("publish on a consumer store = %v, want errReadOnlyStore", err)
	}
	if _, _, err := consumer.GC(RefSet{}); !errors.Is(err, errReadOnlyStore) {
		t.Fatalf("GC on a consumer store = %v, want errReadOnlyStore", err)
	}
}

func TestOpenConsumerRejectsWritableGenerationsDir(t *testing.T) {
	store := newTestPublisher(t)
	publishTarGz(t, store, launcherEntries())
	sealStoreForConsumer(t, store)
	if err := os.Chmod(store.generationsDir(), 0o755); err != nil {
		t.Fatalf("chmod generations: %v", err)
	}
	_, err := OpenConsumer(store.Root(), "")
	if err == nil {
		t.Fatal("expected a writable generations directory to be refused")
	}
	if !strings.Contains(err.Error(), store.generationsDir()) {
		t.Fatalf("error does not name the writable directory: %v", err)
	}
}

func TestWritableByMode(t *testing.T) {
	cases := []struct {
		name       string
		perm       os.FileMode
		ownerUID   uint32
		ownerKnown bool
		euid       uint32
		want       bool
	}{
		{"world writable", 0o777, 1000, true, 1000, true},
		{"world writable for other user", 0o757, 1000, true, 2000, true},
		{"group writable", 0o770, 1000, true, 2000, true},
		{"owner writable same uid", 0o700, 1000, true, 1000, true},
		{"owner writable other uid", 0o700, 1000, true, 2000, false},
		{"read only 0555", 0o555, 1000, true, 1000, false},
		{"read only 0444", 0o444, 1000, true, 1000, false},
		{"unknown owner writable fails closed", 0o700, 0, false, 2000, true},
		{"unknown owner read only", 0o555, 0, false, 2000, false},
		{"no write bits at all", 0o500, 1000, true, 2000, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := writableByMode(testCase.perm, testCase.ownerUID, testCase.ownerKnown, testCase.euid)
			if got != testCase.want {
				t.Fatalf("writableByMode(%04o, uid=%d, known=%v, euid=%d) = %v, want %v",
					testCase.perm, testCase.ownerUID, testCase.ownerKnown, testCase.euid, got, testCase.want)
			}
		})
	}
}

func TestWritableByCurrentUserReadsRealModes(t *testing.T) {
	if !supportsUnixPermissions() {
		t.Skip("unix permission bits are required")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod 0700: %v", err)
	}
	if !writableByCurrentUser(dir) {
		t.Fatal("owner-writable directory must count as writable")
	}
	for _, sealed := range []os.FileMode{0o555, 0o444} {
		if err := os.Chmod(dir, sealed); err != nil {
			t.Fatalf("chmod %04o: %v", sealed, err)
		}
		if writableByCurrentUser(dir) {
			t.Fatalf("directory sealed %04o must not count as writable", sealed)
		}
	}
	if err := os.Chmod(dir, 0o770); err != nil {
		t.Fatalf("chmod 0770: %v", err)
	}
	if !writableByCurrentUser(dir) {
		t.Fatal("group-writable directory must fail closed")
	}
	if err := os.Chmod(dir, 0o707); err != nil {
		t.Fatalf("chmod 0707: %v", err)
	}
	if !writableByCurrentUser(dir) {
		t.Fatal("world-writable directory must count as writable")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("restore mode: %v", err)
	}
	if !writableByCurrentUser(filepath.Join(dir, "absent")) {
		t.Fatal("an unreadable path must fail closed")
	}
}
