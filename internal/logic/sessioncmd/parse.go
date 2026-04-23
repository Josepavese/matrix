package sessioncmd

import "strings"

type Invocation struct {
	Command string
	Args    string
	Input   string
}

func Parse(input string) (Invocation, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return Invocation{}, false
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return Invocation{}, false
	}
	invocation := Invocation{Command: strings.ToLower(fields[0]), Input: trimmed}
	if len(trimmed) > len(fields[0]) {
		invocation.Args = strings.TrimSpace(trimmed[len(fields[0]):])
	}
	return invocation, true
}
