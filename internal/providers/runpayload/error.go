package runpayload

import "fmt"

// ValidationError contains only owner-defined policy messages, never input bytes.
type ValidationError struct{ message string }

func (e *ValidationError) Error() string { return e.message }
func invalidInput(format string, args ...any) error {
	return &ValidationError{message: fmt.Sprintf(format, args...)}
}
