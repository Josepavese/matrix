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
	return filepath.Join(home, "agents")
}

func Ensure(home string) error {
	for _, dir := range []string{"", "bin", "configs", "data", "logs", "artifacts", "agents", "backups", "tmp"} {
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
