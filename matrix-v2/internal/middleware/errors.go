package middleware

import "fmt"

// Error represents a normalized Matrix error
type Error struct {
	Code    string
	Message string
	Op      string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %s (op: %s)", e.Code, e.Message, e.Err.Error(), e.Op)
	}
	return fmt.Sprintf("[%s] %s (op: %s)", e.Code, e.Message, e.Op)
}
