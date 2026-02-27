package fusefs

import (
	"context"
	"fmt"
	"os"

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
	// Add a read-only file named "matrix.txt"
	content := []byte("Welcome to the Matrix V2 Virtual Filesystem! Wake up, Neo...\n")
	f := &fs.MemRegularFile{
		Data: content,
		Attr: fuse.Attr{
			Mode: 0444,
		},
	}

	// Create a child inode for the file and add it to our root
	ch := r.Inode.NewPersistentInode(ctx, f, fs.StableAttr{Mode: fuse.S_IFREG})
	r.Inode.AddChild("matrix.txt", ch, false)
}

// Provider implements middleware.FS
type Provider struct {
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
			AllowOther: false, // Ensures we don't need root perms to mount locally
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
	p.server = server

	// Serve in the background
	go server.Wait()
	return nil
}

// Unmount safely unmounts the filesystem
func (p *Provider) Unmount() error {
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
