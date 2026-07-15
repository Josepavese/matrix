package providerdiag

import "fmt"

type ProcessFailure struct {
	ExitCode int
	Stderr   string
	Err      error
}

func (e *ProcessFailure) Error() string {
	if e == nil {
		return ""
	}
	message := "provider process exited"
	if e.ExitCode >= 0 {
		message += fmt.Sprintf(" with code %d", e.ExitCode)
	}
	if e.Stderr != "" {
		message += ": " + e.Stderr
	} else if e.Err != nil {
		message += ": " + e.Err.Error()
	}
	return message
}

func (e *ProcessFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
