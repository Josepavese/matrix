// Package oscapacity observes the actual workspace filesystem through native
// Linux, macOS and Windows APIs. It never creates files or deletes data.
package oscapacity

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

type Provider struct{}

func New() *Provider { return &Provider{} }

func (*Provider) Observe(path string) (middleware.CapacitySnapshot, error) {
	result := middleware.CapacitySnapshot{Source: runtime.GOOS + ":native", MeasuredAt: time.Now().UTC(), Scope: "runtime_host_workspace", LogicalCPUs: runtime.NumCPU()}
	resolved, err := resolveDirectory(path)
	if err != nil {
		result.Reason = "workspace_unavailable"
		return result, err
	}
	result.Path = resolved
	free, total, volume, err := observeDisk(resolved)
	if err != nil {
		result.Reason = "filesystem_observation_failed"
		return result, err
	}
	result.Available = true
	result.DiskFreeBytes, result.DiskTotalBytes, result.VolumeID = &free, &total, volume
	result.MemoryTotalBytes, result.MemoryFreeBytes, result.MemorySource = observeMemory()
	return result, nil
}

func resolveDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("workspace path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace path must be a directory")
	}
	return resolved, nil
}
