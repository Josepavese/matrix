//go:build windows

package fusefs

import "os/exec"

func configureMountProcess(*exec.Cmd) {}
func platformMountArgs() []string     { return []string{"-o", "FileSecurity=D:P(A;;FRFX;;;OW)"} }
