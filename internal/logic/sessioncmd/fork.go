package sessioncmd

import "strings"

type ForkInvocation struct {
	Target        string
	Async         bool
	RestoreParent bool
	Ephemeral     bool
	CleanupPolicy string
	MakeActive    *bool
	Input         string
}

func ParseFork(args string) ForkInvocation {
	fields := strings.Fields(args)
	invocation := ForkInvocation{}
	targetParts := make([]string, 0, len(fields))
	for i := 0; i < len(fields); i++ {
		token := fields[i]
		if token == "--" {
			invocation.Input = strings.Join(fields[i+1:], " ")
			break
		}
		if !strings.HasPrefix(token, "--") {
			targetParts = append(targetParts, token)
			continue
		}
		i = applyForkToken(&invocation, fields, i)
	}
	invocation.Target = strings.TrimSpace(strings.Join(targetParts, " "))
	return invocation
}

func applyForkToken(invocation *ForkInvocation, fields []string, index int) int {
	token := fields[index]
	switch token {
	case "--async":
		invocation.Async = true
	case "--restore-parent":
		invocation.RestoreParent = true
	case "--ephemeral":
		invocation.Ephemeral = true
	case "--make-active":
		value := true
		invocation.MakeActive = &value
	case "--no-make-active":
		value := false
		invocation.MakeActive = &value
	case "--input":
		invocation.Input = strings.Join(fields[index+1:], " ")
		return len(fields)
	case "--cleanup-policy":
		if index+1 < len(fields) {
			invocation.CleanupPolicy = fields[index+1]
			return index + 1
		}
	default:
		if key, value, ok := strings.Cut(token, "="); ok {
			applyForkFlag(invocation, key, value)
		}
	}
	return index
}

func applyForkFlag(invocation *ForkInvocation, key, value string) {
	switch key {
	case "--input":
		invocation.Input = value
	case "--cleanup-policy":
		invocation.CleanupPolicy = value
	case "--async":
		invocation.Async = parseBoolFlag(value)
	case "--restore-parent":
		invocation.RestoreParent = parseBoolFlag(value)
	case "--ephemeral":
		invocation.Ephemeral = parseBoolFlag(value)
	case "--make-active":
		parsed := parseBoolFlag(value)
		invocation.MakeActive = &parsed
	}
}

func parseBoolFlag(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1", "t", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
