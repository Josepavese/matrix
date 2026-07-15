package agentmgr

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestProbeOnDemandPersistsInitializeFailure(t *testing.T) {
	store := newRegistryMemStorage()
	supervisor := &Supervisor{
		store: store,
		probe: func(context.Context, middleware.ProtocolEndpoint) error {
			return errors.New("provider process exited with code 7")
		},
	}

	supervisor.probeOnDemand(context.Background(), slog.Default(), "codex", middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindACP, Transport: "stdio",
	})

	states, err := LoadRuntimeStates(store)
	if err != nil {
		t.Fatalf("LoadRuntimeStates failed: %v", err)
	}
	state := states["codex"]
	if state.Status != "initialize_failed" || state.Error == "" {
		t.Fatalf("unexpected probe state: %+v", state)
	}
}

func TestProbeOnDemandPersistsReadyOnlyAfterSuccess(t *testing.T) {
	store := newRegistryMemStorage()
	supervisor := &Supervisor{
		store: store,
		probe: func(context.Context, middleware.ProtocolEndpoint) error { return nil },
	}

	supervisor.probeOnDemand(context.Background(), slog.Default(), "codex", middleware.ProtocolEndpoint{
		Kind: middleware.ProtocolKindACP, Transport: "stdio",
	})

	states, err := LoadRuntimeStates(store)
	if err != nil {
		t.Fatalf("LoadRuntimeStates failed: %v", err)
	}
	if state := states["codex"]; state.Status != "ready_on_demand" {
		t.Fatalf("unexpected probe state: %+v", state)
	}
}
