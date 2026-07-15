//go:build linux || darwin

package agentlaunch

import (
	"fmt"
	"strings"
)

func PrepareStdio(command string, args []string, envIsolation bool) (string, []string) {
	if !envIsolation {
		return command, append([]string{}, args...)
	}
	var launch strings.Builder
	launch.WriteString(`export NVM_DIR="$HOME/.nvm"; if [ -s "$NVM_DIR/nvm.sh" ]; then \. "$NVM_DIR/nvm.sh"; fi; `)
	fmt.Fprintf(&launch, "%q", command)
	for _, arg := range args {
		launch.WriteByte(' ')
		fmt.Fprintf(&launch, "%q", arg)
	}
	return "bash", []string{"-c", launch.String()}
}
