//go:build windows

package oscapacity

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type memoryStatus struct {
	Length, Load                                               uint32
	TotalPhysical, AvailablePhysical, TotalPage, AvailablePage uint64
	TotalVirtual, AvailableVirtual, AvailableExtended          uint64
}

func observeMemory() (*uint64, *uint64, string) {
	status := memoryStatus{}
	status.Length = uint32(unsafe.Sizeof(status))
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	result, _, _ := proc.Call(uintptr(unsafe.Pointer(&status)))
	if result == 0 {
		return nil, nil, "unavailable"
	}
	return &status.TotalPhysical, &status.AvailablePhysical, "windows:GlobalMemoryStatusEx"
}
