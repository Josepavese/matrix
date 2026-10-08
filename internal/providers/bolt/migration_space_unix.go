//go:build linux || darwin

package bolt

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func availableMigrationSpace(path string) (diskSpace, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return diskSpace{}, err
	}
	return diskSpace{free: stat.Bavail * uint64(stat.Bsize), volume: fmt.Sprintf("%v", stat.Fsid)}, nil
}
