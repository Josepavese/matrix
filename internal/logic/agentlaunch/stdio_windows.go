//go:build windows

package agentlaunch

func PrepareStdio(command string, args []string, _ bool) (string, []string) {
	return command, append([]string{}, args...)
}
