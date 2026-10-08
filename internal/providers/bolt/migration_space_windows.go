//go:build windows

package bolt

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func availableMigrationSpace(path string) (diskSpace, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return diskSpace{}, err
	}
	var free uint64
	if err := windows.GetDiskFreeSpaceEx(name, &free, nil, nil); err != nil {
		return diskSpace{}, err
	}
	return diskSpace{free: free, volume: strings.ToUpper(filepath.VolumeName(path))}, nil
}
