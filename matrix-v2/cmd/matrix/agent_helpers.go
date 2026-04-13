package main

import goexec "os/exec"

func execLookPath(name string) (string, error) {
	return goexec.LookPath(name)
}
