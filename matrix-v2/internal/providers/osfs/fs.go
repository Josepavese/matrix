package osfs

import (
	"os"

	"github.com/jose/matrix-v2/internal/middleware"
)

// FSProvider implements middleware.FS using native OS calls.
type FSProvider struct{}

// NewFSProvider returns a new FSProvider.
func NewFSProvider() *FSProvider {
	return &FSProvider{}
}

// osFile wraps an *os.File to implement middleware.File.
type osFile struct {
	*os.File
}

// Mount is a no-op for the native OS filesystem provider.
func (p *FSProvider) Mount(dir string) error {
	return nil
}

// Unmount is a no-op for the native OS filesystem provider.
func (p *FSProvider) Unmount() error {
	return nil
}

// CreateDirectory creates a directory at path with default permissions (0755).
func (p *FSProvider) CreateDirectory(path string) error {
	return os.MkdirAll(path, 0755)
}

// RemoveAll removes path and any children it contains.
func (p *FSProvider) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// Stat returns file info for the given path.
func (p *FSProvider) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// MkdirAll creates a directory named path with the specified permissions, along with any necessary parents.
func (p *FSProvider) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// UserHomeDir returns the current user's home directory.
func (p *FSProvider) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

// TempDir returns the default directory used for temporary files.
func (p *FSProvider) TempDir() string {
	return os.TempDir()
}

// Open opens the named file for reading.
func (p *FSProvider) Open(path string) (middleware.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &osFile{File: f}, nil
}

// OpenFile opens the named file with the specified flags and permissions.
func (p *FSProvider) OpenFile(path string, flag int, perm os.FileMode) (middleware.File, error) {
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	return &osFile{File: f}, nil
}

// ReadFile reads the named file and returns its contents.
func (p *FSProvider) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Remove removes the named file or (empty) directory.
func (p *FSProvider) Remove(path string) error {
	return os.Remove(path)
}

// Rename renames (moves) oldpath to newpath.
func (p *FSProvider) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
