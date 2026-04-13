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

func (p *FSProvider) Mount(dir string) error {
	return nil
}

func (p *FSProvider) Unmount() error {
	return nil
}

func (p *FSProvider) CreateDirectory(path string) error {
	return os.MkdirAll(path, 0755)
}

func (p *FSProvider) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (p *FSProvider) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (p *FSProvider) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (p *FSProvider) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

func (p *FSProvider) TempDir() string {
	return os.TempDir()
}

func (p *FSProvider) Open(path string) (middleware.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &osFile{File: f}, nil
}

func (p *FSProvider) OpenFile(path string, flag int, perm os.FileMode) (middleware.File, error) {
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	return &osFile{File: f}, nil
}

func (p *FSProvider) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (p *FSProvider) Remove(path string) error {
	return os.Remove(path)
}

func (p *FSProvider) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
