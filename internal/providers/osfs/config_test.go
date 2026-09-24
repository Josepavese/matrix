package osfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/matrixhome"
)

// newConfigHome points MATRIX_HOME at a fresh PAL home whose configuration
// directory exists, and returns the home's real path. The provider resolves
// symlinks, so a home reached through a symlinked temporary root (macOS /var)
// has to be resolved here as well or the expected root would not compare equal.
func newConfigHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temporary home: %v", err)
	}
	t.Setenv(matrixhome.EnvName, home)
	if err := os.MkdirAll(matrixhome.ConfigsDir(home), 0o700); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	return home
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// chdirTo enters dir for the rest of the test, the way the process runs once
// matrixhome.Configure has entered the PAL home.
func chdirTo(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("change working directory to %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

func assertNotCreated(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the refused write must not create %s (stat error: %v)", path, err)
	}
}

// TestWriteConfigWritesFilesInsideTheConfigDirectory is the legitimate half of
// the containment rule: the configuration directory is exactly where Config_Set
// is supposed to write, for a file that already exists and for one that does not
// exist yet.
func TestWriteConfigWritesFilesInsideTheConfigDirectory(t *testing.T) {
	home := newConfigHome(t)
	// The daemon runs with the PAL home as its working directory
	// (matrixhome.Configure chdirs there), which is how a relative config key has
	// always addressed configs/. Mirror that here so the test pins the call shape
	// Config_Set actually uses.
	chdirTo(t, home)
	provider := NewConfigProvider()

	// An existing configuration file is updated in place.
	existing := filepath.Join(home, "configs", "agents.json")
	if err := os.WriteFile(existing, []byte(`{"agents":[]}`), 0o644); err != nil {
		t.Fatalf("seed existing config: %v", err)
	}
	if err := os.Chmod(existing, 0o644); err != nil {
		t.Fatalf("set legacy config mode: %v", err)
	}
	if err := provider.WriteConfig("configs/agents.json", []byte(`{"agents":["claude"]}`)); err != nil {
		t.Fatalf("an existing config file inside the config directory must stay writable: %v", err)
	}
	if got := readFile(t, existing); got != `{"agents":["claude"]}` {
		t.Fatalf("the config file was not updated, it holds %q", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(existing)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("existing config mode was not tightened: %v %v", info, err)
		}
	}

	// A configuration file that does not exist yet is created.
	fresh := filepath.Join(home, "configs", "telegram.json")
	if err := provider.WriteConfig("configs/telegram.json", []byte(`{"enabled":false}`)); err != nil {
		t.Fatalf("a missing config file inside the config directory must be creatable: %v", err)
	}
	if got := readFile(t, fresh); got != `{"enabled":false}` {
		t.Fatalf("the new config file holds %q", got)
	}

	// The same directory can also be named absolutely; the rule is containment,
	// not "relative paths only".
	absolute := filepath.Join(home, "configs", "absolute.json")
	if err := provider.WriteConfig(absolute, []byte("ok")); err != nil {
		t.Fatalf("an absolute path inside the config directory must be accepted: %v", err)
	}
	if got := readFile(t, absolute); got != "ok" {
		t.Fatalf("the config file named absolutely holds %q", got)
	}

	// The key is a path, so ".." that stays inside the directory must resolve
	// rather than be refused by a string comparison.
	if err := os.MkdirAll(filepath.Join(home, "configs", "nested"), 0o700); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}
	if err := provider.WriteConfig("configs/nested/../app.json", []byte("ok")); err != nil {
		t.Fatalf("a path that resolves inside the config directory must be accepted: %v", err)
	}
	if got := readFile(t, filepath.Join(home, "configs", "app.json")); got != "ok" {
		t.Fatalf("the resolved config file holds %q", got)
	}
}

// TestWriteConfigAcceptsAHomeReachedThroughASymlink keeps the containment check
// from refusing an ordinary setup: MATRIX_HOME may itself be a symlink — on macOS
// a temporary directory lives under one — and a config file inside the directory
// it points at is still inside the configuration directory.
func TestWriteConfigAcceptsAHomeReachedThroughASymlink(t *testing.T) {
	realHome, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temporary home: %v", err)
	}
	if err := os.MkdirAll(matrixhome.ConfigsDir(realHome), 0o700); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	linkedHome := filepath.Join(t.TempDir(), "matrix-home")
	if err := os.Symlink(realHome, linkedHome); err != nil {
		t.Skipf("this platform cannot create symlinks (%v), so a symlinked PAL home cannot be exercised here", err)
	}
	t.Setenv(matrixhome.EnvName, linkedHome)

	if err := NewConfigProvider().WriteConfig("configs/telegram.json", []byte(`{"enabled":false}`)); err != nil {
		t.Fatalf("a config file inside a symlinked PAL home must stay writable: %v", err)
	}
	if got := readFile(t, filepath.Join(realHome, "configs", "telegram.json")); got != `{"enabled":false}` {
		t.Fatalf("the config file holds %q", got)
	}
}

// TestWriteConfigRejectsAnAbsolutePathOutsideTheConfigDirectory covers the
// original finding: an agent names an absolute path, and nothing outside the
// configuration directory may be written.
func TestWriteConfigRejectsAnAbsolutePathOutsideTheConfigDirectory(t *testing.T) {
	home := newConfigHome(t)
	outside := filepath.Join(t.TempDir(), "outside.json")

	err := NewConfigProvider().WriteConfig(outside, []byte("pwned"))
	if err == nil {
		t.Fatalf("an absolute path outside %s must be refused", matrixhome.ConfigsDir(home))
	}
	if !strings.Contains(err.Error(), outside) {
		t.Fatalf("the refusal must name the requested path %q, got %q", outside, err)
	}
	if !strings.Contains(err.Error(), matrixhome.ConfigsDir(home)) {
		t.Fatalf("the refusal must name the allowed root %q, got %q", matrixhome.ConfigsDir(home), err)
	}
	assertNotCreated(t, outside)
}

// TestWriteConfigRejectsRelativePathsThatEscapeWithDotDot keeps a relative key
// from walking out of the configuration directory, whether it stops inside the
// PAL home or leaves it entirely. The test runs from the PAL home and creates the
// directories the escaped paths name, so a write that is not confined succeeds
// here and the refusal cannot be an accident of a missing parent directory.
func TestWriteConfigRejectsRelativePathsThatEscapeWithDotDot(t *testing.T) {
	home := newConfigHome(t)
	chdirTo(t, home)
	if err := os.MkdirAll(filepath.Join(home, "data"), 0o700); err != nil {
		t.Fatalf("create PAL data directory: %v", err)
	}
	provider := NewConfigProvider()

	cases := []struct {
		name    string
		path    string
		escapee string
		explain string
	}{
		{
			name:    "out of configs and into the PAL home",
			path:    "configs/../data/escape.json",
			escapee: filepath.Join(home, "data", "escape.json"),
			explain: "a path that leaves configs/ is outside the configuration directory even when it stays in the PAL home",
		},
		{
			name:    "out of the PAL home",
			path:    "configs/../../escape.json",
			escapee: filepath.Join(filepath.Dir(home), "escape.json"),
			explain: "a path may not climb out of the PAL home",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := provider.WriteConfig(tc.path, []byte("pwned")); err == nil {
				t.Fatalf("%s: %q must be refused", tc.explain, tc.path)
			}
			assertNotCreated(t, tc.escapee)
		})
	}
}

// TestWriteConfigRejectsASymlinkThatPointsOutsideTheConfigDirectory is the reason
// the check runs on the resolved path: the requested string is a plain name under
// configs/, and only following the link shows that the write would land outside.
func TestWriteConfigRejectsASymlinkThatPointsOutsideTheConfigDirectory(t *testing.T) {
	home := newConfigHome(t)
	chdirTo(t, home)
	outsideDir := t.TempDir()
	link := filepath.Join(home, "configs", "link")
	if err := os.Symlink(outsideDir, link); err != nil {
		t.Skipf("this platform cannot create symlinks (%v), so the symlink escape cannot be exercised here", err)
	}
	provider := NewConfigProvider()

	if err := provider.WriteConfig("configs/link/escape.json", []byte("pwned")); err == nil {
		t.Fatalf("a symlink to %s must not carry a write outside the config directory", outsideDir)
	}
	assertNotCreated(t, filepath.Join(outsideDir, "escape.json"))

	// A dangling symlink pointing outside is refused too: os.WriteFile would
	// follow it and create the file it names.
	danglingTarget := filepath.Join(outsideDir, "created.json")
	if err := os.Symlink(danglingTarget, filepath.Join(home, "configs", "dangling")); err != nil {
		t.Fatalf("create dangling symlink: %v", err)
	}
	if err := provider.WriteConfig("configs/dangling", []byte("pwned")); err == nil {
		t.Fatal("a dangling symlink pointing outside the config directory must be refused, not written through")
	}
	assertNotCreated(t, danglingTarget)

	// A symlink that stays inside is legitimate: the rule is about where the write
	// lands, not about refusing every link.
	resolvedTarget := filepath.Join(home, "configs", "real.json")
	if err := os.WriteFile(resolvedTarget, []byte("old"), 0o600); err != nil {
		t.Fatalf("seed symlink target: %v", err)
	}
	if err := os.Symlink(resolvedTarget, filepath.Join(home, "configs", "inner")); err != nil {
		t.Fatalf("create inner symlink: %v", err)
	}
	if err := provider.WriteConfig("configs/inner", []byte("new")); err != nil {
		t.Fatalf("a symlink pointing inside the config directory must be accepted: %v", err)
	}
	if got := readFile(t, resolvedTarget); got != "new" {
		t.Fatalf("the write did not land on the resolved target, which holds %q", got)
	}
}

// TestWriteConfigRejectsASiblingThatSharesTheConfigDirectoryPrefix pins the
// difference between a path element comparison and a string prefix comparison:
// "configs-evil" starts with "configs" and is still not inside it.
func TestWriteConfigRejectsASiblingThatSharesTheConfigDirectoryPrefix(t *testing.T) {
	home := newConfigHome(t)
	lookalike := filepath.Join(home, "configs-evil")
	if err := os.MkdirAll(lookalike, 0o700); err != nil {
		t.Fatalf("create lookalike directory: %v", err)
	}
	target := filepath.Join(lookalike, "app.json")

	if err := NewConfigProvider().WriteConfig(target, []byte("pwned")); err == nil {
		t.Fatalf("%s only shares a prefix with the config directory and must be refused", target)
	}
	assertNotCreated(t, target)
}

// TestConfigWriteRefusalNamesTheRequestedPathAndAllowedRoot checks the message an
// agent or operator receives: it has to say what was refused and where a config
// file may live, otherwise the refusal is not actionable.
func TestConfigWriteRefusalNamesTheRequestedPathAndAllowedRoot(t *testing.T) {
	home := newConfigHome(t)
	chdirTo(t, home)
	root := matrixhome.ConfigsDir(home)
	if err := os.MkdirAll(filepath.Join(home, "configs-evil"), 0o700); err != nil {
		t.Fatalf("create lookalike directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, "data"), 0o700); err != nil {
		t.Fatalf("create PAL data directory: %v", err)
	}

	cases := []struct {
		name string
		path string
	}{
		{"absolute outside", filepath.Join(t.TempDir(), "outside.json")},
		{"parent traversal", "configs/../data/escape.json"},
		{"sibling prefix", filepath.Join(home, "configs-evil", "app.json")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewConfigProvider().WriteConfig(tc.path, []byte("pwned"))
			if err == nil {
				t.Fatalf("%q must be refused", tc.path)
			}
			if !strings.Contains(err.Error(), tc.path) {
				t.Fatalf("the refusal must name the requested path %q, got %q", tc.path, err)
			}
			if !strings.Contains(err.Error(), root) {
				t.Fatalf("the refusal must name the allowed root %q, got %q", root, err)
			}
		})
	}
}
