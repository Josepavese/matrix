// Package matrixhome resolves and prepares the Matrix PAL home.
package matrixhome

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const EnvName = "MATRIX_HOME"

// Names of the PAL home subdirectories the code addresses individually.
const (
	configsDirName = "configs"
	agentsDirName  = "agents"
)

func Configure() (string, error) {
	home, err := Resolve()
	if err != nil {
		return "", err
	}
	if err := Ensure(home); err != nil {
		return "", err
	}
	if err := os.Chdir(home); err != nil {
		return "", fmt.Errorf("failed to enter matrix home %s: %w", home, err)
	}
	return home, nil
}

func Resolve() (string, error) {
	if home := strings.TrimSpace(os.Getenv(EnvName)); home != "" {
		return filepath.Abs(home)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user home: %w", err)
	}
	switch runtime.GOOS {
	case "windows":
		if base := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); base != "" {
			return filepath.Join(base, "Matrix"), nil
		}
		return filepath.Join(userHome, "AppData", "Local", "Matrix"), nil
	case "darwin":
		return filepath.Join(userHome, "Library", "Application Support", "Matrix"), nil
	default:
		if base := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); base != "" {
			return filepath.Join(base, "matrix"), nil
		}
		return filepath.Join(userHome, ".local", "share", "matrix"), nil
	}
}

func AgentsDir(home string) string {
	return filepath.Join(home, agentsDirName)
}

// ConfigsDir returns the directory that holds Matrix configuration files. It is
// the only tree a configuration write may touch, so callers that accept a
// configuration path from outside the process must confine it here.
func ConfigsDir(home string) string {
	return filepath.Join(home, configsDirName)
}

func Ensure(home string) error {
	for _, dir := range []string{"", "bin", configsDirName, "data", "logs", "artifacts", agentsDirName, "backups", "tmp"} {
		path := home
		if dir != "" {
			path = filepath.Join(home, dir)
		}
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("failed to create matrix home directory %s: %w", path, err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("failed to secure matrix home directory %s: %w", path, err)
		}
	}
	return nil
}
