package sessioncleanup

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// accessDeniedError is the Windows shape reported by the issue: taskkill exits
// non-zero and the follow-up TerminateProcess is refused.
func accessDeniedError() error {
	return errors.New("close retained client codex\x00C:\\Users\\rober\\Documents\\Half Pocket: exit status 128\nTerminateProcess: Accesso negato")
}

// fakeProcessProvider stands in for the OS process provider: it answers a
// scripted sequence of termination outcomes and counts how many times the agent
// client was actually asked to die.
type fakeProcessProvider struct {
	outcomes []error
	attempts int
}

func (p *fakeProcessProvider) terminate() error {
	p.attempts++
	if len(p.outcomes) == 0 {
		return nil
	}
	outcome := p.outcomes[0]
	if len(p.outcomes) > 1 {
		p.outcomes = p.outcomes[1:]
	}
	return outcome
}

func newFakeProcessProvider(outcomes ...error) *fakeProcessProvider {
	return &fakeProcessProvider{outcomes: outcomes}
}

func TestIsTerminationAccessDeniedRecognizesWindowsDenial(t *testing.T) {
	if !IsTerminationAccessDenied(accessDeniedError()) {
		t.Fatalf("windows TerminateProcess denial must be classified as retryable")
	}
	if !IsTerminationAccessDenied(errors.New("kill: operation not permitted")) {
		t.Fatalf("POSIX EPERM during teardown must be classified as retryable")
	}
}

func TestIsTerminationAccessDeniedIgnoresUnrelatedFailures(t *testing.T) {
	cases := []error{
		nil,
		errors.New("exit status 128"),
		errors.New("close retained client codex: context deadline exceeded"),
		errors.New("reconcile transport closed"),
	}
	for _, err := range cases {
		if IsTerminationAccessDenied(err) {
			t.Fatalf("must not retry unrelated error: %v", err)
		}
	}
}

// Access denied then success on retry: the teardown succeeds and reports no
// error, so a completed run is not converted into a failure.
func TestRetryTerminationRecoversFromTransientAccessDenial(t *testing.T) {
	provider := newFakeProcessProvider(accessDeniedError(), nil)

	if err := RetryTermination(context.Background(), provider.terminate, nil); err != nil {
		t.Fatalf("transient denial must be retried to success, got %v", err)
	}
	if provider.attempts != 2 {
		t.Fatalf("expected one retry after the denial, got %d attempts", provider.attempts)
	}
}

// Persistent access denied: the retry is bounded and the OS detail survives so
// the caller can name what was refused.
func TestRetryTerminationBoundsPersistentAccessDenial(t *testing.T) {
	provider := newFakeProcessProvider(accessDeniedError())

	err := RetryTermination(context.Background(), provider.terminate, nil)
	if err == nil {
		t.Fatalf("persistent denial must be reported as a failure")
	}
	if provider.attempts != TerminationMaxAttempts {
		t.Fatalf("expected %d bounded attempts, got %d", TerminationMaxAttempts, provider.attempts)
	}
	if !strings.Contains(err.Error(), "Accesso negato") {
		t.Fatalf("final error must keep the OS denial detail, got %q", err)
	}
}

// A non-retryable failure must not be retried at all.
func TestRetryTerminationDoesNotRetryUnrelatedError(t *testing.T) {
	provider := newFakeProcessProvider(errors.New("exit status 128"))

	if err := RetryTermination(context.Background(), provider.terminate, nil); err == nil {
		t.Fatalf("unrelated error must be reported")
	}
	if provider.attempts != 1 {
		t.Fatalf("unrelated error must be attempted once, got %d", provider.attempts)
	}
}

// A cancelled context stops retrying instead of waiting out the backoff.
func TestRetryTerminationStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := newFakeProcessProvider(accessDeniedError())

	if err := RetryTermination(ctx, provider.terminate, nil); err == nil {
		t.Fatalf("cancelled context must not report success")
	}
	if provider.attempts != 1 {
		t.Fatalf("cancelled context must not retry, got %d attempts", provider.attempts)
	}
}
