//go:build windows

package oslog

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows ignores Unix mode bits. Use a protected DACL at creation, and tighten
// existing files before writing. Append-only access preserves concurrent append
// semantics; rotation refuses an unexpected new destination instead of truncating it.
func openPrivateLogFile(path string, truncate bool) (*os.File, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	attrs := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	disposition := uint32(windows.OPEN_ALWAYS)
	if truncate {
		disposition = windows.CREATE_NEW
	}
	access := uint32(windows.FILE_APPEND_DATA | windows.FILE_READ_ATTRIBUTES | windows.WRITE_DAC | windows.READ_CONTROL | windows.SYNCHRONIZE)
	handle, err := windows.CreateFile(name, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, &attrs, disposition, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	dacl, _, err := descriptor.DACL()
	if err == nil {
		err = windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
	}
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}
