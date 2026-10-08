package containersandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/Josepavese/matrix/internal/logic/childenv"
)

// Exec supplies validators the same PAL isolation as agents, without retaining
// stdout/stderr. Exit status comes from the daemon's actual stopped container,
// not the CLI's attach exit (which can also describe a lost connection).
func Exec(ctx context.Context, launch Launch) (code int, err error) {
	owner, err := prepare(ctx, launch)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, owner.Close()) }()
	cmd := exec.CommandContext(ctx, owner.engine, "start", "--attach", "--interactive", owner.name)
	cmd.Env = childenv.Environment()
	cmd.WaitDelay = 2 * time.Second
	_ = cmd.Run()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return stoppedExitCode(ctx, owner)
}

func stoppedExitCode(ctx context.Context, owner *Transport) (int, error) {
	output, err := engineCommand(ctx, owner.engine, childenv.Environment(), "inspect", "--format", "{{json .State}}", owner.name)
	if err != nil {
		return 0, err
	}
	var state struct {
		Running  bool
		Status   string
		ExitCode int
		Error    string
	}
	if json.Unmarshal(output, &state) != nil || state.Running || state.Status != "exited" {
		return 0, fmt.Errorf("validator container did not reach an observed exit")
	}
	if state.Error != "" {
		return 0, fmt.Errorf("validator container execution failed")
	}
	return state.ExitCode, nil
}
