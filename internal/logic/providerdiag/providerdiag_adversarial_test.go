package providerdiag

import (
	"errors"
	"strings"
	"testing"
)

// TestProcessFailureErrorDescribesTheExit keeps diagnostics useful: an operator
// reading one line must learn the exit code and the provider's own words.
func TestProcessFailureErrorDescribesTheExit(t *testing.T) {
	failure := &ProcessFailure{ExitCode: 3, Stderr: "boom", Err: errors.New("signal")}
	message := failure.Error()
	for _, fragment := range []string{"provider process exited", "code 3", "boom"} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("message %q is missing %q", message, fragment)
		}
	}
	if !errors.Is(failure, failure.Err) {
		t.Fatal("the wrapped cause must stay reachable")
	}

	// Stderr wins over the wrapped error because it carries the provider's own
	// explanation; the fallback must still say something.
	fallback := (&ProcessFailure{ExitCode: -1, Err: errors.New("spawn failed")}).Error()
	if !strings.Contains(fallback, "spawn failed") {
		t.Fatalf("the wrapped cause must be reported when stderr is empty: %q", fallback)
	}
	if strings.Contains(fallback, "code") {
		t.Fatalf("an unknown exit code must not be rendered: %q", fallback)
	}

	// A typed nil must not panic: error values travel through interfaces.
	var nilFailure *ProcessFailure
	if nilFailure.Error() != "" || nilFailure.Unwrap() != nil {
		t.Fatal("a nil process failure must degrade quietly")
	}
}

// TestStderrCaptureRespectsItsLimit keeps a chatty provider from growing the
// trace without bound, which is a memory and log-flooding risk.
func TestStderrCaptureRespectsItsLimit(t *testing.T) {
	capture := NewStderrCapture(64, "codex")
	capture.Write([]byte(strings.Repeat("x", 200)))
	if got := capture.Sanitized(); len(got) > 64 {
		t.Fatalf("captured %d bytes, limit was 64", len(got))
	}
	// A non-positive limit disables capture rather than panicking or growing.
	disabled := NewStderrCapture(0, "codex")
	if _, err := disabled.Write([]byte("ignored")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := disabled.Sanitized(); got != "" {
		t.Fatalf("a disabled capture must stay empty, got %q", got)
	}
	// Flushing an empty capture must not log a blank line or panic.
	disabled.Flush()
}
