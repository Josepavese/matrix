package agentstore

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// archiveEntry is one fixture entry for the tar.gz and zip writers below.
type archiveEntry struct {
	name string
	mode int64
	body string
	kind byte // tar.TypeReg when zero
	link string
}

// testArchiver is the deployment archive provider the publisher is injected
// with. The store reuses this implementation instead of a second extractor.
func testArchiver() middleware.Archive {
	return osfs.NewArchiveProvider()
}

func launcherEntries() []archiveEntry {
	return []archiveEntry{
		{name: "opencode", mode: 0o755, body: "#!/bin/sh\necho opencode\n"},
		{name: "lib/data.txt", mode: 0o644, body: "artifact data"},
	}
}

func writeTestTarGz(t *testing.T, path string, entries []archiveEntry) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: entry.mode, Typeflag: entry.kind, Linkname: entry.link, Size: int64(len(entry.body))}
		if header.Typeflag == 0 {
			header.Typeflag = tar.TypeReg
		}
		if header.Mode == 0 {
			header.Mode = 0o644
		}
		if header.Typeflag == tar.TypeSymlink {
			header.Size = 0
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write tar header %q: %v", entry.name, err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := tarWriter.Write([]byte(entry.body)); err != nil {
				t.Fatalf("write tar entry %q: %v", entry.name, err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
}

func writeTestZip(t *testing.T, path string, entries []archiveEntry) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zipWriter := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		header.SetMode(os.FileMode(entry.mode))
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", entry.name, err)
		}
		if _, err := writer.Write([]byte(entry.body)); err != nil {
			t.Fatalf("write zip entry %q: %v", entry.name, err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

func digestOfFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sampleGeneration(digest string) Generation {
	return Generation{
		AgentID:  "opencode",
		Version:  "1.2.3",
		Platform: "linux/amd64",
		Artifact: "artifact.tar.gz",
		Launcher: "opencode",
		Args:     []string{"acp"},
	}
}

// newTestPublisher opens a publisher store under a temporary directory and
// registers the cleanup that restores write permission on sealed trees before
// t.TempDir removes the tree.
func newTestPublisher(t *testing.T) *Store {
	t.Helper()
	root := filepath.Join(t.TempDir(), "store")
	store, err := OpenPublisher(root, "")
	if err != nil {
		t.Fatalf("open publisher: %v", err)
	}
	t.Cleanup(func() {
		_ = makeTreeWritable(filepath.Join(store.root, blobsDirName))
		_ = makeTreeWritable(filepath.Join(store.root, generationsDirName))
		_ = makeTreeWritable(filepath.Join(store.root, tmpDirName))
		_ = os.Chmod(store.root, 0o700)
	})
	return store
}

func publishTarGz(t *testing.T, store *Store, entries []archiveEntry) (Generation, string, string) {
	t.Helper()
	archive := filepath.Join(t.TempDir(), "artifact.tar.gz")
	writeTestTarGz(t, archive, entries)
	digest := digestOfFile(t, archive)
	published, err := store.PublishArchive(testArchiver(), archive, digest, sampleGeneration(digest))
	if err != nil {
		t.Fatalf("publish archive: %v", err)
	}
	return published, digest, archive
}

// sealStoreForConsumer makes the store layout read-only for the current user,
// which is the deployment step a consumer open requires.
func sealStoreForConsumer(t *testing.T, store *Store) {
	t.Helper()
	if !supportsUnixPermissions() {
		t.Skip("unix permission bits are required to seal a store")
	}
	dirs := store.layoutDirs()
	for _, dir := range dirs {
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatalf("seal %s: %v", dir, err)
		}
	}
	t.Cleanup(func() {
		for _, dir := range dirs {
			_ = os.Chmod(dir, 0o755)
		}
	})
}

func supportsUnixPermissions() bool {
	return os.PathSeparator == '/'
}

func assertSealedTree(t *testing.T, tree string) {
	t.Helper()
	if !supportsUnixPermissions() {
		return
	}
	info, err := os.Stat(tree)
	if err != nil {
		t.Fatalf("stat sealed tree: %v", err)
	}
	if info.Mode().Perm()&0o222 != 0 {
		t.Fatalf("tree root is still writable: %04o", info.Mode().Perm())
	}
	err = filepath.WalkDir(tree, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o222 != 0 {
			t.Errorf("published entry %s is still writable: %04o", path, info.Mode().Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk sealed tree: %v", err)
	}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func hexDigest(fill string) string {
	return strings.Repeat(fill, 64)
}

func oldTimestamp() time.Time {
	return time.Now().Add(-2 * time.Hour)
}
