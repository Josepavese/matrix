//go:build linux

package oscapacity

import (
	"golang.org/x/sys/unix"
)

func observeMemory() (*uint64, *uint64, string) {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return nil, nil, "unavailable"
	}
	total, free := info.Totalram*uint64(info.Unit), info.Freeram*uint64(info.Unit)
	return &total, &free, "linux:sysinfo_physical_free_excludes_reclaimable_cache"
}
