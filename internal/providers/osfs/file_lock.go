package osfs

import (
	"errors"
	"io"
	"os"
)

type exclusiveFileLock struct{ file *os.File }

func (l exclusiveFileLock) Close() error {
	return errors.Join(unlockExclusiveFile(l.file), l.file.Close())
}

// TryExclusiveFileLock returns an owned handle, or nil if another process
// holds the lock. Closing it releases the lock. The path must stay in place
// so that all participants continue to lock the same file.
func TryExclusiveFileLock(path string) (io.Closer, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	acquired, err := tryExclusiveFileLock(file)
	if err != nil || !acquired {
		return nil, errors.Join(err, file.Close())
	}
	return exclusiveFileLock{file: file}, nil
}
