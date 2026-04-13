// Package fusefs implements a FUSE-based virtual filesystem provider.
package fusefs

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jose/matrix-v2/internal/middleware"
)

// syntheticRoot is a simple root node for the Matrix Virtual FS
type syntheticRoot struct {
	fs.Inode
}

// OnAdd is called when the inode is added to the system
func (r *syntheticRoot) OnAdd(ctx context.Context) {
	content := []byte("Welcome to the Matrix V2 Virtual Filesystem! Wake up, Neo...\n")
	f := &fs.MemRegularFile{
		Data: content,
		Attr: fuse.Attr{
			Mode: 0444,
		},
	}

	ch := r.NewPersistentInode(ctx, f, fs.StableAttr{Mode: fuse.S_IFREG})
	r.AddChild("matrix.txt", ch, false)
}

// Provider implements middleware.FS
type Provider struct {
	mu     sync.Mutex
	server *fuse.Server
}

// NewProvider returns a new fuse provider
func NewProvider() *Provider {
	return &Provider{}
}

// Mount mounts the synthetic filesystem to dir
func (p *Provider) Mount(dir string) error {
	root := &syntheticRoot{}
	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			AllowOther: false,
			Debug:      false,
			Options:    []string{"default_permissions"},
		},
	}

	server, err := fs.Mount(dir, root, opts)
	if err != nil {
		return &middleware.Error{
			Code:    "ERR_FUSE_MOUNT",
			Message: fmt.Sprintf("Failed to mount fuse at %s", dir),
			Op:      "fusefs.Mount",
			Err:     err,
		}
	}
	p.mu.Lock()
	p.server = server
	p.mu.Unlock()

	go server.Wait()
	return nil
}

// Unmount safely unmounts the filesystem
func (p *Provider) Unmount() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.server == nil {
		return nil
	}
	err := p.server.Unmount()
	if err != nil {
		return &middleware.Error{
			Code:    "ERR_FUSE_UNMOUNT",
			Message: "Failed to unmount fuse filesystem",
			Op:      "fusefs.Unmount",
			Err:     err,
		}
	}
	return nil
}

// CreateDirectory ensures the directory exists (MkdirAll)
func (p *Provider) CreateDirectory(path string) error {
	return os.MkdirAll(path, 0755)
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
	return &osFileWrapper{File: f}, nil
}

// OpenFile opens the named file with the specified flags and permissions.
func (p *Provider) OpenFile(path string, flag int, perm os.FileMode) (middleware.File, error) {
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	return &osFileWrapper{File: f}, nil
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

// osFile wraps *os.File to implement middleware.File.
type osFileWrapper struct {
	*os.File
}
