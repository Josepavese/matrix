//go:build !linux && !darwin

package fusefs

import (
	"fmt"

	"github.com/jose/matrix-v2/internal/middleware"
)

// Mount reports that the synthetic FUSE filesystem is unavailable on this OS.
func (p *Provider) Mount(dir string) error {
	return &middleware.Error{
		Code:    "ERR_FUSE_UNSUPPORTED",
		Message: fmt.Sprintf("FUSE mount is not supported on this platform for %s", dir),
		Op:      "fusefs.Mount",
	}
}

// Unmount is a no-op when FUSE is unavailable.
func (p *Provider) Unmount() error {
	p.mu.Lock()
	p.server = nil
	p.mounted = false
	p.mu.Unlock()
	return nil
}
