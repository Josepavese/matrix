//go:build linux || darwin

package fusefs

import (
	"os"
	"os/exec"
)

func configureMountProcess(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
}
func platformMountArgs() []string { return []string{"--umask", "077"} }
