package runactivity

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ----------------------------------------------------------------------------
// Adversarial watchdog suite
// ----------------------------------------------------------------------------

// TestDurationSecondsNeverOverflows keeps a large but legal timeout from
// wrapping into a negative or absurdly short deadline.
func TestDurationSecondsNeverOverflows(t *testing.T) {
	cases := map[string]int{
		"zero":                0,
		"negative":            -5,
		"one second":          1,
		"one hour":            3600,
		"very large":          9223372037,
		"largest int32 range": 2147483647,
	}
	for name, seconds := range cases {
		duration := DurationSeconds(seconds)
		if seconds <= 0 {
			if duration != 0 {
				t.Fatalf("%s: expected a disabled timeout, got %s", name, duration)
			}
			continue
		}
		if duration <= 0 {
			t.Fatalf("%s: timeout wrapped to %s (seconds=%d)", name, duration, seconds)
		}
		// A clamped value must still be far in the future, not a few
		// nanoseconds.
		if duration < time.Second {
			t.Fatalf("%s: timeout collapsed to %s", name, duration)
		}
	}
}

// TestDurationSecondsClampsAboveTheMaximum documents the clamp boundary.
func TestDurationSecondsClampsAboveTheMaximum(t *testing.T) {
	huge := DurationSeconds(int(maxDurationSeconds) + 1000)
	ceiling := DurationSeconds(int(maxDurationSeconds))
	if huge != ceiling {
		t.Fatalf("expected clamping to %s, got %s", ceiling, huge)
	}
	if ceiling <= 0 {
		t.Fatalf("clamped duration must stay positive, got %s", ceiling)
	}
}

// TestContextWithZeroTimeoutIsCancellable proves a disabled timeout still
// yields a usable context rather than a nil one.
func TestContextWithZeroTimeoutIsCancellable(t *testing.T) {
	ctx, cancel := Context(context.Background(), 0)
	if ctx == nil || cancel == nil {
		t.Fatal("a disabled timeout must still return a context and cancel func")
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		t.Fatal("a disabled timeout must not set a deadline")
	}
	cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("cancel must be observable, got %v", ctx.Err())
	}
}

// TestPredicatesTolerateNilContext keeps a caller mistake from panicking the
// run loop.
func TestPredicatesTolerateNilContext(t *testing.T) {
	//nolint:staticcheck // the nil context is the case under test
	if IsDeadline(nil, errors.New("boom"), time.Second) {
		t.Fatal("an unrelated error is not a deadline")
	}
	//nolint:staticcheck // the nil context is the case under test
	if !IsDeadline(nil, context.DeadlineExceeded, time.Second) {
		t.Fatal("an explicit deadline error must be recognised even without a context")
	}
	//nolint:staticcheck // the nil context is the case under test
	if IsDeadline(nil, context.DeadlineExceeded, 0) {
		t.Fatal("a disabled timeout must not classify errors as deadlines")
	}
	//nolint:staticcheck // the nil context is the case under test
	if IsContextCancelled(nil, nil) {
		t.Fatal("nil error and nil context are not a cancellation")
	}
	//nolint:staticcheck // the nil context is the case under test
	if !IsContextCancelled(nil, context.Canceled) {
		t.Fatal("a cancelled error must be recognised without a context")
	}
}

// TestPredicatesRecogniseContextState pins the positive cases.
func TestPredicatesRecogniseContextState(t *testing.T) {
	timedOut, cancelTimeout := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelTimeout()
	<-timedOut.Done()
	if !IsDeadline(timedOut, nil, time.Second) {
		t.Fatal("an expired context must be recognised as a deadline")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if !IsContextCancelled(cancelled, nil) {
		t.Fatal("a cancelled context must be recognised")
	}
	if IsDeadline(cancelled, nil, time.Second) {
		t.Fatal("a plain cancellation is not a deadline")
	}
}

// TestDurationSecondsIsMonotonic keeps the conversion sane across the range.
func TestDurationSecondsIsMonotonic(t *testing.T) {
	previous := time.Duration(0)
	for _, seconds := range []int{1, 2, 5, 30, 60, 600, 3600, 86400} {
		current := DurationSeconds(seconds)
		if current <= previous {
			t.Fatalf("DurationSeconds(%d)=%s is not above %s", seconds, current, previous)
		}
		previous = current
	}
}
