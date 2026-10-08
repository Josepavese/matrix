package runactivity

import (
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestNoticeUnknownActivityRearmsAndStops(t *testing.T) {
	called := make(chan struct{}, 8)
	n, stop := WithNotice(time.Millisecond, nil, func() { called <- struct{}{} })
	defer stop()
	wait := func() {
		t.Helper()
		select {
		case <-called:
		case <-time.After(time.Second):
			t.Fatal("inactivity was never delivered")
		}
	}
	wait()
	n.OnThought(middleware.ThoughtUpdate{})
	wait()
	stop()
	n.OnThought(middleware.ThoughtUpdate{})
	select {
	case <-called:
		t.Fatal("stopped observer still woke the supervisor")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestNoticeDisabledPreservesNotifier(t *testing.T) {
	n, stop := WithNotice(0, nil, func() { t.Error("unconfigured notice fired") })
	defer stop()
	if n != nil {
		t.Fatal("default turn was decorated")
	}
}
