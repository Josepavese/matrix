//go:build linux || darwin

package workspacegrant

import (
	"fmt"
	"os"
	"syscall"
)

func requireOwnedDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("path is not owned by the Matrix user: %s", path)
	}
	return nil
}
