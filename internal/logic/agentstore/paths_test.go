package agentstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRootRejectsRelativeAndEmptyPaths(t *testing.T) {
	for _, root := range []string{"", "   ", "relative/store"} {
		if _, err := ValidateRoot(root, ""); err == nil {
			t.Fatalf("expected root %q to be rejected", root)
		}
	}
}

func TestValidateRootAcceptsCreatableAbsolutePath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "store")
	resolved, err := ValidateRoot(root, "")
	if err != nil {
		t.Fatalf("validate root: %v", err)
	}
	if !filepath.IsAbs(resolved) {
		t.Fatalf("resolved root is not absolute: %q", resolved)
	}
	if filepath.Base(resolved) != "store" {
		t.Fatalf("resolved root lost its name: %q", resolved)
	}
}

func TestValidateRootRejectsStoreInsidePalHome(t *testing.T) {
	home := t.TempDir()
	for _, root := range []string{home, filepath.Join(home, "store"), filepath.Join(home, "a", "b")} {
		if _, err := ValidateRoot(root, home); err == nil {
			t.Fatalf("expected store at %s to be rejected", root)
		} else if !strings.Contains(err.Error(), "outside the PAL home") {
			t.Fatalf("unexpected error for %s: %v", root, err)
		}
	}
}

func TestValidateRootRejectsFileRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store-file")
	if err := os.WriteFile(root, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := ValidateRoot(root, ""); err == nil {
		t.Fatal("expected a regular file root to be rejected")
	}
}

func TestValidateRootResolvesSymlinkContainment(t *testing.T) {
	if !supportsUnixPermissions() {
		t.Skip("symlinks are required")
	}
	home := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(outside, "store-link")
	if err := os.Symlink(home, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if _, err := ValidateRoot(link, home); err == nil {
		t.Fatal("expected a symlink into the PAL home to be rejected")
	}
	realStore := filepath.Join(outside, "real-store")
	if err := os.MkdirAll(realStore, 0o700); err != nil {
		t.Fatalf("create real store: %v", err)
	}
	homeLink := filepath.Join(home, "store-link")
	if err := os.Symlink(realStore, homeLink); err != nil {
		t.Fatalf("create home symlink: %v", err)
	}
	resolved, err := ValidateRoot(homeLink, home)
	if err != nil {
		t.Fatalf("symlink to an outside store should be accepted: %v", err)
	}
	if resolved != realStore {
		t.Fatalf("root not resolved: got %q want %q", resolved, realStore)
	}
}

func TestValidateDigestRejectsPrefixAndMalformed(t *testing.T) {
	good := hexDigest("a")
	got, err := validateDigest(good)
	if err != nil || got != good {
		t.Fatalf("valid digest rejected: %q %v", got, err)
	}
	bad := []string{"", "sha256:" + good, strings.ToUpper(good), good[:63], good + "0", strings.Repeat("g", 64), " " + good}
	for _, digest := range bad {
		if _, err := validateDigest(digest); err == nil {
			t.Fatalf("expected digest %q to be rejected", digest)
		}
	}
	if _, err := validateDigest("sha256:" + good); !strings.Contains(err.Error(), "sha256:") {
		t.Fatalf("prefix error does not name the prefix: %v", err)
	}
}

func TestReferencePathIsUnderHomeConfigs(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "pal")
	want := filepath.Join(home, "configs", "agent_references.json")
	if got := ReferencePath(home); got != want {
		t.Fatalf("ReferencePath = %q, want %q", got, want)
	}
}

func TestIsWithinRejectsSiblingPrefix(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "store")
	if isWithin(filepath.Join(string(filepath.Separator), "store-other"), base) {
		t.Fatal("sibling with a shared prefix must not count as within")
	}
	if !isWithin(filepath.Join(base, "nested"), base) {
		t.Fatal("nested path must count as within")
	}
	if !isWithin(base, base) {
		t.Fatal("the base itself must count as within")
	}
}
