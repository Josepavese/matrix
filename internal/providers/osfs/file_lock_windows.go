package osfs

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

const lockLength = 1

func tryExclusiveFileLock(file *os.File) (bool, error) {
	var start windows.Overlapped
	err := windows.LockFileEx(windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, lockLength, 0, &start)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}

func unlockExclusiveFile(file *os.File) error {
	var start windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, lockLength, 0, &start)
}
