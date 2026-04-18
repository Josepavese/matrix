//go:build windows

package zedacp

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
)

func prepareStdioCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		return terminateStdioCommand(cmd)
	}
}

func terminateStdioCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	if pid <= 0 {
		return nil
	}
	if err := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run(); err != nil {
		if killErr := cmd.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return errors.Join(err, killErr)
		}
		return err
	}
	return nil
}
