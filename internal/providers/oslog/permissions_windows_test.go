//go:build windows

package oslog

import (
	"strings"
	"testing"

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
	if err != nil || !strings.Contains(sd.String(), user.User.Sid.String()) {
		t.Fatal("log grant is not the current account", err)
	}
}
