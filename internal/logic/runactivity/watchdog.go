package runactivity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

var ErrTimeout = errors.New("run activity timeout")

type Timeout struct {
	After time.Duration
	fired atomic.Bool
}

type notifier struct {
	inner   middleware.ThoughtNotifier
	mu      sync.Mutex
	timer   *time.Timer
	after   time.Duration
	stopped bool
	dueAt   time.Time
}

func WithTimeout(ctx context.Context, after time.Duration, inner middleware.ThoughtNotifier) (context.Context, middleware.ThoughtNotifier, *Timeout, func()) {
	state := &Timeout{After: after}
	if after <= 0 {
		return ctx, inner, state, func() {}
	}
	runCtx, cancel := context.WithCancel(ctx)
	wrapped := &notifier{inner: inner, after: after, dueAt: time.Now().Add(after)}
	wrapped.timer = time.AfterFunc(after, func() {
		wrapped.mu.Lock()
		defer wrapped.mu.Unlock()
		if !wrapped.stopped && !time.Now().Before(wrapped.dueAt) {
			state.fired.Store(true)
			cancel()
		}
	})
	stop := func() {
		wrapped.mu.Lock()
		if wrapped.timer != nil {
			wrapped.stopped = true
			wrapped.timer.Stop()
		}
		wrapped.mu.Unlock()
		cancel()
	}
	return runCtx, wrapped, state, stop
}

func (n *notifier) OnThought(update middleware.ThoughtUpdate) {
	n.mu.Lock()
	if n.timer != nil && !n.stopped {
		n.dueAt = time.Now().Add(n.after)
		n.timer.Reset(n.after)
	}
	n.mu.Unlock()
	if n.inner != nil {
		n.inner.OnThought(update)
	}
}

func (n *notifier) SetHeader(agentID, agentSessionID string) {
	if n.inner != nil {
		n.inner.SetHeader(agentID, agentSessionID)
	}
}

func (n *notifier) FormattedHeader() string {
	if n.inner == nil {
		return ""
	}
	return n.inner.FormattedHeader()
}

// OnModelSelection forwards provider model evidence to the wrapped notifier.
// The watchdog decorates a notifier without changing what that notifier can do:
// a decorator that silently drops an optional capability makes the wrapped
// implementation unreachable, and the ACP adapter publishes model attestation
// only through this interface.
func (n *notifier) OnModelSelection(selection middleware.ModelSelection) {
	if n.inner == nil {
		return
	}
	if forwarder, ok := n.inner.(middleware.ModelSelectionNotifier); ok {
		forwarder.OnModelSelection(selection)
	}
}

// turnStopReasonReporter is the capability a turn uses to report what a provider
// said ended it. It is declared here as the method set rather than imported from
// the package that stores the reason, so this decorator stays independent of the
// run store and forwards any implementation of the capability.
type turnStopReasonReporter interface {
	OnTurnStopReason(stopReason string)
}

// OnTurnStopReason forwards the reason a provider reported for ending its turn,
// for the same reason OnModelSelection is forwarded above: the watchdog
// decorates a notifier without changing what that notifier can do, and a
// decorator that silently drops a capability makes the wrapped implementation
// unreachable. Dropping this one does not degrade gracefully — the run records
// "unreported" for a reason the provider did state, so enabling an activity
// timeout would silently discard the provider's own word.
func (n *notifier) OnTurnStopReason(stopReason string) {
	if n.inner == nil {
		return
	}
	if reporter, ok := n.inner.(turnStopReasonReporter); ok {
		reporter.OnTurnStopReason(stopReason)
	}
}

func IsTimeout(state *Timeout, err error) bool {
	return state != nil && state.fired.Load() && errors.Is(err, context.Canceled)
}

func Error(state *Timeout) error {
	if state == nil || state.After <= 0 {
		return ErrTimeout
	}
	return fmt.Errorf("%w: no agent activity for %s", ErrTimeout, state.After)
}

func IsTimeoutError(err error) bool {
	return errors.Is(err, ErrTimeout)
}

// maxDurationSeconds keeps a large integer from overflowing the nanosecond
// multiplication into a negative or nonsensically short timeout.
const maxDurationSeconds = int64(1<<62) / int64(time.Second)

func DurationSeconds(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	if int64(seconds) > maxDurationSeconds {
		return time.Duration(maxDurationSeconds) * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func Context(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

func IsDeadline(ctx context.Context, err error, timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func IsContextCancelled(ctx context.Context, err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	return ctx != nil && errors.Is(ctx.Err(), context.Canceled)
}
