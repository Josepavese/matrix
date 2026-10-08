package agentmgr

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/middleware"
)

type liveStartProcess struct {
	startRecordingProcess
	count atomic.Int32
	done  chan struct{}
}

func TestFailedSupervisionCannotBeRestartedByRouting(t *testing.T) {
	store := memstore.New()
	proc := &liveStartProcess{done: make(chan struct{})}
	s := &Supervisor{store: store, proc: proc, lifetime: context.Background(), running: map[string]*AgentProcess{}}
	close(proc.done)
	crashes := maxFastCrashes - 1
	if retry := s.handleExit(context.Background(), slog.Default(), supervisedRun{AgentID: "failed", Handle: liveStartHandle{proc.done}, StartedAt: time.Now()}, &crashes); retry {
		t.Fatal("crash-loop boundary requested another restart")
	}
	states, err := LoadRuntimeStates(store)
	if err != nil || states["failed"].Status != "crash_loop" {
		t.Fatalf("crash-loop state missing: %+v %v", states, err)
	}
	// The watchdog records an intrinsic give-up when it returns. Routing must
	// respect it instead of creating another watchdog on every incoming run.
	s.failed = map[string]bool{"failed": true}
	if err := s.ensureSupervision("failed", AgentConfig{Command: "peer"}); err == nil {
		t.Fatal("routing restarted a failed supervisor")
	}
	if proc.count.Load() != 0 {
		t.Fatal("new child launched after crash-loop give-up")
	}
}

func (p *liveStartProcess) HasExecutable(string) bool { return true }
func (p *liveStartProcess) Start(middleware.CommandSpec) (middleware.ProcessHandle, error) {
	p.count.Add(1)
	return liveStartHandle{p.done}, nil
}

type liveStartHandle struct{ done <-chan struct{} }

func (h liveStartHandle) Wait() error { <-h.done; return nil }
func (h liveStartHandle) Kill() error { return nil }
func (h liveStartHandle) GetPID() int { return 99 }

func TestLateSupervisedAgentStartsOnceWithoutDaemonRestart(t *testing.T) {
	store := memstore.New()
	reg, err := NewRegistry(nil, store)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	proc := &liveStartProcess{done: make(chan struct{})}
	t.Cleanup(func() { cancel(); close(proc.done) })
	s := NewSupervisor(proc, freePortNetwork{}, store, reg)
	if err := s.StartAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := agentcfg.SaveEntry(store, "late", agentcfg.Entry{Config: AgentConfig{Kind: "acp", Transport: "ws", Command: "peer"}}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			ep, err := s.GetAgentEndpoint("late")
			if err != nil || ep.Address != "127.0.0.1:45711" {
				t.Errorf("late endpoint: %+v %v", ep, err)
			}
		})
	}
	wg.Wait()
	if proc.count.Load() != 1 {
		t.Fatalf("parallel routing launched %d children", proc.count.Load())
	}
}

func TestStoppedDaemonRejectsLateSupervision(t *testing.T) {
	store := memstore.New()
	reg, err := NewRegistry(nil, store)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := NewSupervisor(&liveStartProcess{done: make(chan struct{})}, freePortNetwork{}, store, reg)
	if err := s.StartAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ensureSupervision("late", AgentConfig{Command: "peer"}); err == nil {
		t.Fatal("stopped daemon accepted a new child")
	}
}
