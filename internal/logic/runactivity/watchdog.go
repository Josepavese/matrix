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
	inner middleware.ThoughtNotifier
	mu    sync.Mutex
	timer *time.Timer
	after time.Duration
}

func WithTimeout(ctx context.Context, after time.Duration, inner middleware.ThoughtNotifier) (context.Context, middleware.ThoughtNotifier, *Timeout, func()) {
	state := &Timeout{After: after}
	if after <= 0 {
		return ctx, inner, state, func() {}
	}
	runCtx, cancel := context.WithCancel(ctx)
	wrapped := &notifier{inner: inner, after: after}
	wrapped.timer = time.AfterFunc(after, func() {
		state.fired.Store(true)
		cancel()
	})
	stop := func() {
		wrapped.mu.Lock()
		if wrapped.timer != nil {
			wrapped.timer.Stop()
		}
		wrapped.mu.Unlock()
		cancel()
	}
	return runCtx, wrapped, state, stop
}

func (n *notifier) OnThought(update middleware.ThoughtUpdate) {
	n.mu.Lock()
	if n.timer != nil {
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

func DurationSeconds(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
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
	return timeout > 0 && (errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded)
}

func IsContextCancelled(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled)
}
