// Package filesystem provides virtual filesystem mount/unmount logic.
package filesystem

import (
	"log/slog"

	"github.com/Josepavese/matrix/internal/middleware"
)

// Manager handles virtual filesystem operations
type Manager struct {
	fs middleware.FS
}

// NewManager creates a new Filesystem manager via PAL provider
func NewManager(fs middleware.FS) *Manager {
	return &Manager{fs: fs}
}

// MountVirtualFS ensures the directory exists and mounts the pseudo-FS
func (m *Manager) MountVirtualFS(mountPoint string) error {
	log := slog.With("component", "filesystem")

	if err := m.fs.CreateDirectory(mountPoint); err != nil {
		return &middleware.Error{
			Code:    "ERR_FS_MKDIR",
			Message: "Failed to create mount point directory",
			Op:      "filesystem.MountVirtualFS",
			Err:     err,
		}
	}

	log.Info("mounting virtual fs", "event", "mount_start", "path", mountPoint)
	return m.fs.Mount(mountPoint)
}

// UnmountVirtualFS cleanly unmounts the system
func (m *Manager) UnmountVirtualFS() error {
	slog.With("component", "filesystem").Info("unmounting virtual fs", "event", "unmount_start")
	return m.fs.Unmount()
}
