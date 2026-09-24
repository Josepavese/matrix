//go:build linux || darwin

package agentlaunch

import (
	"strings"
)

func shellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func PrepareStdio(command string, args []string, envIsolation bool) (string, []string) {
	if !envIsolation {
		return command, append([]string{}, args...)
	}
	var launch strings.Builder
	launch.WriteString(`export NVM_DIR="$HOME/.nvm"; if [ -s "$NVM_DIR/nvm.sh" ]; then \. "$NVM_DIR/nvm.sh"; fi; `)
	launch.WriteString(shellLiteral(command))
	for _, arg := range args {
		launch.WriteByte(' ')
		launch.WriteString(shellLiteral(arg))
	}
	return "bash", []string{"-c", launch.String()}
}
