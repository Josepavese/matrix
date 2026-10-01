package agentmgr

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
)

// startRecordingProcess records every launch the supervisor attempts and never
// produces a real child.
type startRecordingProcess struct {
	specs []middleware.CommandSpec
}

func (p *startRecordingProcess) Exec(middleware.CommandSpec) ([]byte, error) {
	return nil, errors.New("not used by this test")
}

func (p *startRecordingProcess) ExecSeparate(context.Context, middleware.CommandSpec) (*middleware.ExecResult, error) {
	return nil, errors.New("not used by this test")
}

func (p *startRecordingProcess) Start(spec middleware.CommandSpec) (middleware.ProcessHandle, error) {
	p.specs = append(p.specs, spec)
	return nil, errors.New("this test must never reach a real launch")
}

func (p *startRecordingProcess) StartPiped(middleware.CommandSpec) (middleware.PipedProcess, error) {
	return nil, errors.New("not used by this test")
}

func (p *startRecordingProcess) RunPrivileged(middleware.CommandSpec) ([]byte, error) {
	return nil, errors.New("not used by this test")
}

func (p *startRecordingProcess) HasExecutable(string) bool { return false }

func (p *startRecordingProcess) SpawnPTY() error { return errors.New("not used by this test") }

// freePortNetwork hands out a port and does nothing else.
type freePortNetwork struct{}

func (freePortNetwork) Listen(string, string) (middleware.ClosableListener, error) {
	return nil, errors.New("not used by this test")
}

func (freePortNetwork) Download(context.Context, string, string) error {
	return errors.New("not used by this test")
}

func (freePortNetwork) FetchJSON(context.Context, string, interface{}) error {
	return errors.New("not used by this test")
}

func (freePortNetwork) GetFreePort() (int, error) { return 45711, nil }

func (freePortNetwork) Fetch(context.Context, string) ([]byte, error) {
	return nil, errors.New("not used by this test")
}

func (freePortNetwork) PostJSON(context.Context, string, interface{}) ([]byte, int, error) {
	return nil, 0, errors.New("not used by this test")
}

func (freePortNetwork) CanDial(string) bool { return false }

// TestSupervisorRefusesAnUnusableDeclaredProcessCwd pins the daemon half of the
// process cwd guardrail: an agent whose declared process cwd cannot be honored
// is never started anywhere else. Starting it in the daemon's own directory
// would be the silent substitution that pinned agents to the wrong checkout in
// the first place, and it would leave the runtime reporting a healthy child.
func TestSupervisorRefusesAnUnusableDeclaredProcessCwd(t *testing.T) {
	store := newRegistryMemStorage()
	proc := &startRecordingProcess{}
	supervisor := &Supervisor{
		proc:    proc,
		net:     freePortNetwork{},
		store:   store,
		running: map[string]*AgentProcess{},
	}

	missing := filepath.Join(t.TempDir(), "absent-checkout")
	cfg := AgentConfig{
		Command: "/bin/sleep",
		Args:    []string{"30"},
		Env:     []string{agentlaunch.ProcessCwdEnv + "=" + missing},
		Active:  boolPtr(true),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	supervisor.watchdog(ctx, "mimo", cfg)

	if len(proc.specs) != 0 {
		t.Fatalf("an unusable declared process cwd still launched a child: %#v", proc.specs)
	}
	states, err := LoadRuntimeStates(store)
	if err != nil {
		t.Fatalf("LoadRuntimeStates: %v", err)
	}
	state, ok := states["mimo"]
	if !ok {
		t.Fatal("the refusal was not recorded in runtime state")
	}
	if state.Status != "process_cwd_invalid" {
		t.Fatalf("runtime status = %q, want process_cwd_invalid", state.Status)
	}
	if state.Error == "" {
		t.Fatal("the refusal must say what to fix")
	}
	_ = slog.Default()
}

func boolPtr(value bool) *bool { return &value }
