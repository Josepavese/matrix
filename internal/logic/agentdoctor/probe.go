package agentdoctor

import (
	"context"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
)

type CommandProbe struct {
	OK       bool
	ExitCode int
	Error    string
}

func ProbeCommand(command string, args []string, env []string, envIsolation bool) CommandProbe {
	probeArgs := append([]string{}, args...)
	probeArgs = append(probeArgs, "--help")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	res, err := execprovider.NewProvider().ExecSeparate(ctx, middleware.CommandSpec{
		Runner: command, Args: probeArgs, Env: env, EnvIsolation: envIsolation,
	})
	if err != nil {
		return CommandProbe{Error: trimProbeError(err.Error())}
	}
	if res == nil {
		return CommandProbe{Error: "empty probe result"}
	}
	probe := CommandProbe{ExitCode: res.ExitCode}
	if res.ExitCode == 0 {
		probe.OK = true
		return probe
	}
	probe.Error = trimProbeError(string(append(res.Stderr, res.Stdout...)))
	return probe
}

func trimProbeError(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\x00", "\\0"))
	if raw == "" {
		return "probe failed"
	}
	if len(raw) > 400 {
		return raw[:400]
	}
	return raw
}
