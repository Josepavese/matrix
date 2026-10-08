//go:build linux || darwin

package oscapacity

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func observeDisk(path string) (uint64, uint64, string, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, "", err
	}
	return stat.Bavail * uint64(stat.Bsize), stat.Blocks * uint64(stat.Bsize), fmt.Sprintf("unix:%v", stat.Fsid), nil
}
