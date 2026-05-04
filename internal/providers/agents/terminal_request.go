package agents

import (
	"encoding/json"
	"fmt"

	"github.com/Josepavese/matrix/internal/middleware"
)

type terminalCreateRequest struct {
	Command         string
	Args            []string
	Env             []string
	WorkDir         string
	OutputByteLimit int
}

func (h *defaultRequestHandler) parseTerminalCreateRequest(params json.RawMessage) (terminalCreateRequest, error) {
	var raw struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Cwd     string   `json:"cwd"`
		Env     []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"env"`
		OutputByteLimit int `json:"outputByteLimit"`
	}
	if err := json.Unmarshal(params, &raw); err != nil {
		return terminalCreateRequest{}, fmt.Errorf("invalid terminal/create params: %w", err)
	}
	if raw.Command == "" {
		return terminalCreateRequest{}, fmt.Errorf("terminal/create requires a 'command' field")
	}
	workDir := h.cwd
	if raw.Cwd != "" {
		if resolved := h.resolvePath(raw.Cwd); resolved != "" {
			workDir = resolved
		}
	}
	env := make([]string, 0, len(raw.Env))
	for _, item := range raw.Env {
		if item.Name == "" {
			continue
		}
		env = append(env, item.Name+"="+item.Value)
	}
	return terminalCreateRequest{
		Command:         raw.Command,
		Args:            raw.Args,
		Env:             env,
		WorkDir:         workDir,
		OutputByteLimit: raw.OutputByteLimit,
	}, nil
}

func (r terminalCreateRequest) commandSpec() middleware.CommandSpec {
	return middleware.CommandSpec{Runner: r.Command, Args: r.Args, Env: r.Env, Dir: r.WorkDir}
}
