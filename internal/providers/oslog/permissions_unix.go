//go:build !windows

package oslog

import "os"

func openPrivateLogFile(path string, truncate bool) (*os.File, error) {
	flags := os.O_CREATE | os.O_APPEND | os.O_WRONLY
	if truncate {
		flags |= os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, 0600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}
