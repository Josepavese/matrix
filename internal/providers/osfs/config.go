package osfs

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/matrixhome"
)

// ConfigProvider implements middleware.ConfigReader using the native OS filesystem.
// This is the platform-specific provider for config reading.
type ConfigProvider struct{}

// NewConfigProvider creates a new ConfigProvider.
func NewConfigProvider() *ConfigProvider {
	return &ConfigProvider{}
}

// ReadConfig reads the file at the given path and returns its contents.
func (p *ConfigProvider) ReadConfig(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteConfig writes the provided data to the file at the given path, which must
// resolve inside the Matrix configuration directory.
//
// The path is not an operator choice: `Config_Set` hands this provider a path
// taken from an agent tool call, so without confinement an agent could write any
// file the operator's user can write. Reads stay unrestricted because every
// caller names the configuration it wants.
func (p *ConfigProvider) WriteConfig(path string, data []byte) error {
	target, err := configWriteTarget(path)
	if err != nil {
		return err
	}
	// Channel configuration can carry identifiers and tokens, and Matrix is
	// single-user: there is no reader that needs group or world access.
	return os.WriteFile(target, data, 0o600)
}

// configWriteTarget resolves the file a config write would touch and refuses it
// unless the result lies inside the Matrix configuration directory.
//
// The decision is made on the resolved path, not on the raw string, because a raw
// string does not say where the write lands: "configs/../data/x" merely looks
// contained, and "configs/link" is contained or not depending on what link points
// at. Resolving first expands every ".." and every symlink, so the path that is
// checked is the path that is written.
func configWriteTarget(path string) (string, error) {
	home, err := matrixhome.Resolve()
	if err != nil {
		return "", fmt.Errorf("cannot locate the Matrix home for config path %q: %w", path, err)
	}
	root, err := filepath.Abs(matrixhome.ConfigsDir(home))
	if err != nil {
		return "", fmt.Errorf("cannot resolve the Matrix configuration directory for config path %q: %w", path, err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("cannot resolve the Matrix configuration directory %s for config path %q: %w", root, path, err)
	}
	// A relative config path is relative to the PAL home, which is where the
	// process runs and where `configs/` lives; home resolution deliberately
	// ignores the working directory.
	requested := path
	if !filepath.IsAbs(requested) {
		requested = filepath.Join(home, requested)
	}
	absRequested, err := filepath.Abs(requested)
	if err != nil {
		return "", fmt.Errorf("cannot resolve config path %q: %w", path, err)
	}
	resolved, err := resolveConfigSymlinks(absRequested)
	if err != nil {
		return "", fmt.Errorf("cannot resolve config path %q: %w", path, err)
	}
	if !isConfigPathInside(realRoot, resolved) {
		err := fmt.Errorf("refusing to write config path %q: it resolves to %s, outside the Matrix configuration directory %s; config paths must stay inside that directory", path, resolved, realRoot)
		slog.Warn("config write refused: path resolves outside the configuration directory", "path", path, "root", realRoot, "resolved", resolved)
		return "", err
	}
	return resolved, nil
}

// resolveConfigSymlinks returns path with every symbolic link expanded, walking
// one element at a time. A single EvalSymlinks call over the whole path would
// fail on a file that does not exist yet, and would hide a dangling symlink that
// os.WriteFile would follow, so existing elements are resolved individually and
// a symlink that cannot be resolved is refused rather than written through.
func resolveConfigSymlinks(path string) (string, error) {
	volume := filepath.VolumeName(path)
	rest := strings.TrimPrefix(path[len(volume):], string(filepath.Separator))
	current := volume + string(filepath.Separator)
	for _, part := range strings.Split(rest, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		next := filepath.Join(current, part)
		info, err := os.Lstat(next)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			// A configuration file may not exist yet, and a missing element cannot
			// be a symlink, so the remaining elements keep the names as given.
			current = next
		case err != nil:
			return "", fmt.Errorf("cannot inspect %s: %w", next, err)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := filepath.EvalSymlinks(next)
			if err != nil {
				return "", fmt.Errorf("refusing to follow symlink %s: %w", next, err)
			}
			current = link
		default:
			current = next
		}
	}
	return current, nil
}

// isConfigPathInside reports whether resolved is root or a descendant of it. Both
// paths must already be resolved. It compares path elements instead of string
// prefixes, so a sibling directory that merely shares the root's prefix is not
// mistaken for a child.
func isConfigPathInside(root, resolved string) bool {
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
