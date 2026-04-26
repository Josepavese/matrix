package agents

import (
	"encoding/json"
	"fmt"

	"github.com/Josepavese/matrix/internal/middleware"
)

type terminalCreateRequest struct {
	Command string
	Args    []string
	WorkDir string
}

func (h *defaultRequestHandler) parseTerminalCreateRequest(params json.RawMessage) (terminalCreateRequest, error) {
	var raw struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Cwd     string   `json:"cwd"`
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
	return terminalCreateRequest{Command: raw.Command, Args: raw.Args, WorkDir: workDir}, nil
}

func (r terminalCreateRequest) commandSpec() middleware.CommandSpec {
	return middleware.CommandSpec{Runner: r.Command, Args: r.Args, Dir: r.WorkDir}
}
