//go:build windows

package zedacpstdio

import (
	"errors"
	"os"
	"os/exec"
	"strconv"

	"github.com/Josepavese/matrix/internal/logic/childenv"
)

func prepareCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return terminateCommand(cmd) }
}

// taskkillCommand is the system tool that ends a process tree, started with the
// shared child allowlist instead of the daemon's environment: nothing about
// killing a process needs the operator's keys, and a child of Matrix is never
// handed what it does not need.
func taskkillCommand(pid int) *exec.Cmd {
	kill := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	kill.Env = childenv.Environment()
	return kill
}

func terminateCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	if err := taskkillCommand(cmd.Process.Pid).Run(); err != nil {
		if killErr := cmd.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return errors.Join(err, killErr)
		}
		return err
	}
	return nil
}
