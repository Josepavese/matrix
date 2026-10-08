//go:build windows

package oslog

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func assertPrivateLogPermissions(t *testing.T, path string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount != 1 {
		t.Fatal("log ACL must have one explicit owner grant", err)
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("log DACL inherits permissions", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(acl, 0, &ace); err != nil {
		t.Fatal(err)
	}
	granted := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !windows.EqualSid(granted, user.User.Sid) || ace.Mask != windows.ACCESS_MASK(windows.STANDARD_RIGHTS_REQUIRED|windows.SYNCHRONIZE|0x1ff) || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		t.Fatal("log ACL does not grant only the exact current user")
	}
}
