package sessioncleanup

import (
	"context"
	"strings"
	"time"
)

// Access-denied from the OS during process teardown is normally transient: the
// process is still exiting and the kernel refuses the handle for a moment. It
// must be retried a bounded number of times before it is treated as a failure.
const (
	TerminationMaxAttempts = 3
	TerminationBackoff     = 50 * time.Millisecond
)

// IsTerminationAccessDenied reports whether err is an OS refusal to terminate a
// process (Windows "TerminateProcess: Accesso negato" / "Access is denied",
// POSIX EPERM). It deliberately does not match exit-status text alone: a process
// that already exited is not a denial.
func IsTerminationAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "access is denied") ||
		strings.Contains(msg, "accesso negato") ||
		strings.Contains(msg, "access denied") {
		return true
	}
	if strings.Contains(msg, "permission denied") || strings.Contains(msg, "operation not permitted") {
		return strings.Contains(msg, "terminate") ||
			strings.Contains(msg, "kill") ||
			strings.Contains(msg, "close retained client")
	}
	return false
}

// RetryTermination runs attempt and re-runs it while retryable reports that the
// returned error is a transient termination denial and that another pass is
// safe, up to TerminationMaxAttempts with a short linear backoff. The final error
// is returned unchanged so callers keep the OS detail. A nil retryable retries
// every termination denial.
func RetryTermination(ctx context.Context, attempt func() error, retryable func(error) bool) error {
	if retryable == nil {
		retryable = IsTerminationAccessDenied
	}
	var err error
	for tries := 1; tries <= TerminationMaxAttempts; tries++ {
		err = attempt()
		if !retryable(err) {
			return err
		}
		if tries == TerminationMaxAttempts {
			return err
		}
		if !sleepBeforeRetry(ctx) {
			return err
		}
	}
	return err
}

func sleepBeforeRetry(ctx context.Context) bool {
	if ctx == nil {
		time.Sleep(TerminationBackoff)
		return true
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(TerminationBackoff):
		return true
	}
}
