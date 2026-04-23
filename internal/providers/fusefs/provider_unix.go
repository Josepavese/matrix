//go:build linux || darwin

package fusefs

import (
	"context"
	"fmt"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// syntheticRoot is a simple root node for the Matrix Virtual FS.
type syntheticRoot struct {
	fs.Inode
}

// OnAdd is called when the inode is added to the system.
func (r *syntheticRoot) OnAdd(ctx context.Context) {
	content := []byte("Welcome to the Matrix Virtual Filesystem! Wake up, Neo...\n")
	file := &fs.MemRegularFile{
		Data: content,
		Attr: fuse.Attr{Mode: 0o444},
	}

	child := r.NewPersistentInode(ctx, file, fs.StableAttr{Mode: fuse.S_IFREG})
	r.AddChild("matrix.txt", child, false)
}

// Mount mounts the synthetic filesystem to dir.
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
	p.mounted = true
	p.mu.Unlock()

	go server.Wait()
	return nil
}

// Unmount safely unmounts the filesystem.
func (p *Provider) Unmount() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.mounted || p.server == nil {
		return nil
	}
	if err := p.server.Unmount(); err != nil {
		return &middleware.Error{
			Code:    "ERR_FUSE_UNMOUNT",
			Message: "Failed to unmount fuse filesystem",
			Op:      "fusefs.Unmount",
			Err:     err,
		}
	}
	p.server = nil
	p.mounted = false
	return nil
}
