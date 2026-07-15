//go:build windows

package zedacpstdio

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
)

func prepareCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return terminateCommand(cmd) }
}

func terminateCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	if err := exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run(); err != nil {
		if killErr := cmd.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return errors.Join(err, killErr)
		}
		return err
	}
	return nil
}
