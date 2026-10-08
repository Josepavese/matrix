//go:build windows

package oscapacity

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

func observeDisk(path string) (uint64, uint64, string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, "", err
	}
	var available, total uint64
	if err := windows.GetDiskFreeSpaceEx(name, &available, &total, nil); err != nil {
		return 0, 0, "", err
	}
	volume := make([]uint16, 32768)
	if err := windows.GetVolumePathName(name, &volume[0], uint32(len(volume))); err != nil {
		return 0, 0, "", err
	}
	root := windows.UTF16ToString(volume)
	rootName, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return 0, 0, "", err
	}
	guid := make([]uint16, 32768)
	if err := windows.GetVolumeNameForVolumeMountPoint(rootName, &guid[0], uint32(len(guid))); err == nil {
		return available, total, "windows:" + strings.ToUpper(windows.UTF16ToString(guid)), nil
	}
	var serial uint32
	if err := windows.GetVolumeInformation(rootName, nil, 0, &serial, nil, nil, nil, 0); err != nil {
		return 0, 0, "", err
	}
	return available, total, fmt.Sprintf("windows:%08x:%s", serial, strings.ToUpper(root)), nil
}
