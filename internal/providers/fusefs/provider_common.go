package fusefs

import (
	"io/fs"
	"os"
	"sync"

	"github.com/Josepavese/matrix/internal/middleware"
)

// Provider implements middleware.FS.
// Mount/unmount behavior is platform-specific and lives in build-tagged files.
type Provider struct {
	view       fs.FS
	driverPath string
	mu         sync.Mutex
	server     unmountable
	mounted    bool
}

type unmountable interface {
	Unmount() error
}

// NewProvider returns a new fuse provider.
func NewProvider() *Provider {
	return &Provider{}
}

// WithView selects the read-only semantic projection mounted by the PAL driver.
func (p *Provider) WithView(view fs.FS) *Provider        { p.view = view; return p }
func (p *Provider) WithDriverPath(path string) *Provider { p.driverPath = path; return p }

// CreateDirectory ensures the directory exists (MkdirAll).
func (p *Provider) CreateDirectory(path string) error {
	return os.MkdirAll(path, 0o755)
}

// RemoveAll removes path and any children.
func (p *Provider) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// Stat returns file info for the given path.
func (p *Provider) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// MkdirAll creates directories along the path.
func (p *Provider) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// UserHomeDir returns the current user's home directory.
func (p *Provider) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

// TempDir returns the temporary directory path.
func (p *Provider) TempDir() string {
	return os.TempDir()
}

// Open opens the named file for reading.
func (p *Provider) Open(path string) (middleware.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return osFileWrapper{File: f}, nil
}

// OpenFile opens the named file with the specified flags and permissions.
func (p *Provider) OpenFile(path string, flag int, perm os.FileMode) (middleware.File, error) {
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	return osFileWrapper{File: f}, nil
}

// ReadFile reads the named file and returns its contents.
func (p *Provider) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Remove removes the named file or empty directory.
func (p *Provider) Remove(path string) error {
	return os.Remove(path)
}

// Rename renames oldpath to newpath.
func (p *Provider) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

// osFileWrapper wraps *os.File to implement middleware.File.
type osFileWrapper struct {
	*os.File
}
