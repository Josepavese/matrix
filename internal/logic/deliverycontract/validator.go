package deliverycontract

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/Josepavese/matrix/internal/logic/childenv"
)

const (
	defaultValidatorTimeout = 30 * time.Second
	maxValidatorTimeout     = 5 * time.Minute
)

// runValidatorCommand is the seam through which a validator reaches a real
// process. It is a variable so the timeout and exit-code paths can be exercised
// deterministically; one test also runs a real command through it, so the seam
// cannot drift away from the process it stands for.
var runValidatorCommand = runValidatorProcess

func checkValidator(ctx context.Context, workspace string, validator Validator) Check {
	check := Check{Target: "validator"}
	timeout := validatorTimeout(validator)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	code, err := runValidatorCommand(runCtx, workspace, validator.Command)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return check.as(CheckError, fmt.Sprintf("the validator did not finish within %s", timeout))
		}
		return check.as(CheckError, "the validator could not be run: "+err.Error())
	}
	if code != 0 {
		return check.as(CheckFailed, fmt.Sprintf("the validator exited with code %d", code))
	}
	return check.as(CheckPassed, "the validator exited with code 0")
}

func validatorTimeout(validator Validator) time.Duration {
	if validator.TimeoutSeconds <= 0 {
		return defaultValidatorTimeout
	}
	timeout := time.Duration(validator.TimeoutSeconds) * time.Second
	if timeout > maxValidatorTimeout {
		return maxValidatorTimeout
	}
	return timeout
}

// runValidatorProcess runs the caller's argv in the run workspace. The working
// directory is the run's, never the server's, and no shell is introduced: the
// argv is passed through as written, so a shell only appears when the caller
// wrote one into the command itself.
//
// The command's output is deliberately not captured. Only the exit code leaves
// this function. A validator that could write into the run trace would be a side
// channel for content Matrix never asked for and cannot redact, which is the
// same reason the acceptance verdict never carries a transcript.
func runValidatorProcess(ctx context.Context, workspace string, argv []string) (int, error) {
	if len(argv) == 0 {
		return 0, errors.New("no command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = workspace
	// Caller-supplied code does not inherit the daemon's environment: it holds
	// the operator's keys and the workspace is exactly where a validator writes.
	// The allowlist lives in internal/logic/childenv, shared with every other
	// child Matrix starts for a decision of its own.
	cmd.Env = childenv.Environment()
	err := cmd.Run()
	if runErr := ctx.Err(); runErr != nil {
		return 0, runErr
	}
	if err == nil {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	return 0, err
}
