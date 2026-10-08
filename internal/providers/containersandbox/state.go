package containersandbox

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// StateDirectory is stable across policy changes and process restarts, and
// separate across agent identities and physical workspaces. Never erase it when
// closing a process: it holds useful remote sessions and provider state.
func StateDirectory(base, workspace, identity, command string) string {
	key := sha256.Sum256([]byte(workspace + "\x00" + identity + "\x00" + command))
	return filepath.Join(base, fmt.Sprintf("matrix-%x", key[:16]))
}

func prepareStateDirectory(base, workspace, identity, command string) (string, error) {
	path := StateDirectory(base, workspace, identity, command)
	if err := os.MkdirAll(path, 0700); err != nil {
		return "", fmt.Errorf("sandbox persistent state directory unavailable")
	}
	// Refuse a pre-existing symlink that would widen the explicit state mount.
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("sandbox state namespace is not a physical directory")
	}
	return path, nil
}
