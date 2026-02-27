package middleware_test

import (
	"errors"
	"testing"

	"github.com/jose/matrix-v2/internal/middleware"
)

// Verify middleware.Error satisfies the error interface
func TestError_Format(t *testing.T) {
	err := &middleware.Error{
		Code:    "ERR_TEST",
		Message: "something went wrong",
		Op:      "test.op",
		Err:     errors.New("root cause"),
	}

	msg := err.Error()
	if msg == "" {
		t.Errorf("Expected non-empty error message")
	}
	var target *middleware.Error
	if !errors.As(err, &target) {
		t.Errorf("Expected error to be unwrappable as middleware.Error")
	}
}

// Contract test: verify any middleware.Error has required fields
func TestError_WithoutUnderlying(t *testing.T) {
	err := &middleware.Error{
		Code:    "ERR_STANDALONE",
		Message: "no root cause",
		Op:      "test.op",
	}
	msg := err.Error()
	if msg == "" {
		t.Errorf("Expected non-empty message even without underlying error")
	}
}
