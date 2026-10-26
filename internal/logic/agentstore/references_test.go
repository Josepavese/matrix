package agentstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleReference(store, digest, version string) Reference {
	return Reference{
		Store:              store,
		Digest:             digest,
		TreeManifestSHA256: hexDigest("d"),
		Launcher:           "opencode",
		Version:            version,
		Platform:           "linux/amd64",
		InstalledAt:        time.Unix(1700000000, 0).UTC(),
	}
}

func TestReadReferencesMissingIsEmpty(t *testing.T) {
	refs, err := ReadReferences(t.TempDir())
	if err != nil {
		t.Fatalf("read missing references: %v", err)
	}
	if refs.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %d, want %d", refs.SchemaVersion, SchemaVersion)
	}
	if refs.Agents == nil {
		t.Fatal("agents map must not be nil")
	}
	if len(refs.Agents) != 0 {
		t.Fatalf("agents = %+v, want empty", refs.Agents)
	}
}

func TestWriteReferenceMergesAndPreservesOthers(t *testing.T) {
	home := t.TempDir()
	alpha := sampleReference("/store", hexDigest("a"), "1.0.0")
	beta := sampleReference("/store", hexDigest("b"), "2.0.0")
	if err := WriteReference(home, "alpha", alpha); err != nil {
		t.Fatalf("write alpha: %v", err)
	}
	if err := WriteReference(home, "beta", beta); err != nil {
		t.Fatalf("write beta: %v", err)
	}

	refs, err := ReadReferences(home)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if refs.SchemaVersion != SchemaVersion || len(refs.Agents) != 2 {
		t.Fatalf("references = %+v", refs)
	}
	if refs.Agents["alpha"] != alpha || refs.Agents["beta"] != beta {
		t.Fatalf("references lost fields: %+v", refs.Agents)
	}

	updated := alpha
	updated.Version = "1.5.0"
	if err := WriteReference(home, "alpha", updated); err != nil {
		t.Fatalf("update alpha: %v", err)
	}
	refs, err = ReadReferences(home)
	if err != nil {
		t.Fatalf("read after update: %v", err)
	}
	if len(refs.Agents) != 2 {
		t.Fatalf("update dropped another agent: %+v", refs.Agents)
	}
	if refs.Agents["alpha"].Version != "1.5.0" {
		t.Fatalf("alpha version = %q, want 1.5.0", refs.Agents["alpha"].Version)
	}
	if refs.Agents["beta"] != beta {
		t.Fatal("beta reference changed during an alpha update")
	}
}

func TestWriteReferenceUsesPrivatePermissions(t *testing.T) {
	if !supportsUnixPermissions() {
		t.Skip("unix permission bits are required")
	}
	home := t.TempDir()
	if err := WriteReference(home, "alpha", sampleReference("/store", hexDigest("a"), "1.0.0")); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	info, err := os.Stat(ReferencePath(home))
	if err != nil {
		t.Fatalf("stat references: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("references mode = %04o, want 0600", info.Mode().Perm())
	}
	configs, err := os.Stat(filepath.Dir(ReferencePath(home)))
	if err != nil {
		t.Fatalf("stat configs: %v", err)
	}
	if configs.Mode().Perm() != 0o700 {
		t.Fatalf("configs mode = %04o, want 0700", configs.Mode().Perm())
	}
}

func TestRemoveReference(t *testing.T) {
	home := t.TempDir()
	if err := RemoveReference(home, "absent"); err != nil {
		t.Fatalf("removing an absent reference must not fail: %v", err)
	}
	if _, err := os.Stat(ReferencePath(home)); !os.IsNotExist(err) {
		t.Fatalf("removing an absent reference created a file: %v", err)
	}
	if err := WriteReference(home, "alpha", sampleReference("/store", hexDigest("a"), "1.0.0")); err != nil {
		t.Fatalf("write alpha: %v", err)
	}
	if err := WriteReference(home, "beta", sampleReference("/store", hexDigest("b"), "2.0.0")); err != nil {
		t.Fatalf("write beta: %v", err)
	}
	if err := RemoveReference(home, "alpha"); err != nil {
		t.Fatalf("remove alpha: %v", err)
	}
	refs, err := ReadReferences(home)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(refs.Agents) != 1 || refs.Agents["beta"].Digest != hexDigest("b") {
		t.Fatalf("references after remove = %+v", refs.Agents)
	}
	if err := RemoveReference(home, "alpha"); err != nil {
		t.Fatalf("removing an already absent entry must not fail: %v", err)
	}
}

func TestReadReferencesRejectsMalformedJSON(t *testing.T) {
	home := t.TempDir()
	configs := filepath.Join(home, "configs")
	if err := os.MkdirAll(configs, 0o700); err != nil {
		t.Fatalf("create configs: %v", err)
	}
	if err := os.WriteFile(ReferencePath(home), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write malformed references: %v", err)
	}
	if _, err := ReadReferences(home); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected malformed references to be rejected: %v", err)
	}
}

func TestWriteReferenceRejectsEmptyHome(t *testing.T) {
	if err := WriteReference("", "alpha", Reference{}); err == nil {
		t.Fatal("expected an empty PAL home to be rejected")
	}
	if err := WriteReference(t.TempDir(), "", Reference{}); err == nil {
		t.Fatal("expected an empty agent id to be rejected")
	}
	if err := RemoveReference("", "alpha"); err == nil {
		t.Fatal("expected an empty PAL home to be rejected on removal")
	}
	if _, err := ReadReferences(""); err == nil {
		t.Fatal("expected an empty PAL home to be rejected on read")
	}
}

func TestLoadRefSetUnion(t *testing.T) {
	firstHome := t.TempDir()
	secondHome := t.TempDir()
	emptyHome := t.TempDir()
	if err := WriteReference(firstHome, "alpha", sampleReference("/store", hexDigest("a"), "1.0.0")); err != nil {
		t.Fatalf("write alpha: %v", err)
	}
	if err := WriteReference(secondHome, "beta", sampleReference("/store", hexDigest("b"), "2.0.0")); err != nil {
		t.Fatalf("write beta: %v", err)
	}
	if err := WriteReference(secondHome, "no-digest", sampleReference("/store", "", "2.1.0")); err != nil {
		t.Fatalf("write empty digest: %v", err)
	}

	refs, err := LoadRefSet([]string{firstHome, secondHome, emptyHome})
	if err != nil {
		t.Fatalf("load ref set: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("ref set = %+v, want two digests", refs)
	}
	for _, digest := range []string{hexDigest("a"), hexDigest("b")} {
		if _, ok := refs[digest]; !ok {
			t.Fatalf("ref set misses %s", digest)
		}
	}
}

func TestLoadRefSetRejectsUnreadableHome(t *testing.T) {
	home := t.TempDir()
	configs := filepath.Join(home, "configs")
	if err := os.MkdirAll(configs, 0o700); err != nil {
		t.Fatalf("create configs: %v", err)
	}
	if err := os.WriteFile(ReferencePath(home), []byte("{"), 0o600); err != nil {
		t.Fatalf("write malformed references: %v", err)
	}
	if _, err := LoadRefSet([]string{home}); err == nil {
		t.Fatal("expected malformed references to fail the ref set load")
	}
}
