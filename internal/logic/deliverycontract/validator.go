package deliverycontract

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
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

type ValidatorRunner func(context.Context, string, Validator) (int, error)

func validateValidatorSandbox(validator *Validator) error {
	if validator == nil || validator.Sandbox == nil {
		return nil
	}
	return agentlaunch.ValidateContainerSandbox(*validator.Sandbox)
}

func checkValidatorWithRunner(ctx context.Context, workspace string, validator Validator, runner ValidatorRunner) Check {
	check := Check{Target: "validator"}
	timeout := validatorTimeout(validator)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	code, err := runRequestedValidator(runCtx, workspace, validator, runner)
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
//
// CONTAINMENT, stated as a limit and not as a defence: a validator is confined
// by nothing beyond the operating-system user the daemon runs as. The environment
// allowlist above keeps the daemon's keys out of the child, but the child can
// still read and write anywhere that user can — inside and outside the run
// workspace, including the rest of the host. The working directory is set for the
// validator's convenience, not to fence it in. Running a delivery contract is
// therefore trusting the caller's command as much as running any other child
// process, and an operator who needs a fence needs an OS-level sandbox that this
// package does not provide.
// ResolveValidatorBinary reports which binary the daemon will actually execute.
// It is exported because the audit record needs the same answer the child
// process will get, and two implementations of "what will run" would eventually
// disagree. It is best-effort on purpose: a command that cannot be resolved is
// still attempted, so the failure surfaces on the exit path rather than as a
// refusal here.
//
// The answer stays out of the acceptance verdict on purpose. The verdict answers
// "was the delivery accepted" and carries no caller-supplied text; what ran
// belongs to the audit record, which is a separate question with a separate
// reader.
func ResolveValidatorBinary(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	resolved, err := exec.LookPath(argv[0])
	if err != nil {
		return ""
	}
	return resolved
}

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

func runRequestedValidator(ctx context.Context, workspace string, validator Validator, runner ValidatorRunner) (int, error) {
	if validator.Sandbox == nil {
		return runValidatorCommand(ctx, workspace, validator.Command)
	}
	if runner == nil {
		return 0, errors.New("requested validator sandbox runner unavailable")
	}
	return runner(ctx, workspace, validator)
}
