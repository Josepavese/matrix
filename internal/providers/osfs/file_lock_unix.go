//go:build linux || darwin || dragonfly || freebsd || netbsd || openbsd

package osfs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryExclusiveFileLock(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func unlockExclusiveFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
