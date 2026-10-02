package runapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/middleware"
)

// liveRegistryResolver resolves through a real agentmgr registry, which is what
// the daemon's resolver does: `run.go` hands the run API its supervisor, and the
// supervisor answers from this registry.
//
// The stdio branch below mirrors Supervisor.GetAgentEndpoint, whose own behaviour
// for an agent enabled after startup is asserted in agentmgr with the real
// supervisor (TestTheRunPathResolvesAnAgentEnabledAfterStartup). What is measured
// here is the surface the operator met: the 409.
type liveRegistryResolver struct {
	registry *agentmgr.Registry
}

func (r liveRegistryResolver) GetAgentEndpoint(agentID string) (middleware.ProtocolEndpoint, error) {
	cfg, err := r.registry.Get(agentID)
	if err != nil {
		return middleware.ProtocolEndpoint{}, err
	}
	endpoint := middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKind(cfg.Kind),
		Transport: cfg.Transport,
		Command:   cfg.Command,
		Args:      cfg.Args,
		Env:       cfg.Env,
	}
	if endpoint.Kind == middleware.ProtocolKindACP && endpoint.Transport == "stdio" {
		return endpoint, nil
	}
	if !cfg.IsActive() {
		return middleware.ProtocolEndpoint{}, middleware.ErrAgentNotFound
	}
	return endpoint, nil
}

// TestAnAgentEnabledAfterTheDaemonStartedIsNotRefusedWithA409 is the acceptance
// criterion of the hot-enable window stated as the operator met it: the daemon
// was already serving when the agent was enabled, and the next run carrying a
// model_id must not be told that the agent is not an ACP agent — the answer that
// sent them to restart the daemon.
func TestAnAgentEnabledAfterTheDaemonStartedIsNotRefusedWithA409(t *testing.T) {
	store := memstore.New()
	enabled := true
	seed := agentcfg.Entry{Config: agentcfg.Config{Command: "codex-acp", Kind: "acp", Transport: "stdio", Active: &enabled}}
	if err := agentcfg.SaveEntry(store, "codex", seed); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}
	registry, err := agentmgr.NewRegistry(nil, store)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	mux := http.NewServeMux()
	server := NewServer(&runTestRouter{}).WithTraceStorage(memstore.New()).WithEndpointResolver(
		liveRegistryResolver{registry: registry},
	)
	server.RegisterRoutes(mux)

	// The daemon is up and its registry has been read. The agent arrives now.
	late := agentcfg.Entry{Config: agentcfg.Config{Command: "late-agent-acp", Kind: "acp", Transport: "stdio", Active: &enabled}}
	if err := agentcfg.SaveEntry(store, "late-agent", late); err != nil {
		t.Fatalf("SaveEntry(late-agent): %v", err)
	}

	body := `{"channel_id":"ch","input":"do it","agent_id":"late-agent","model_id":"minimax-m3"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, RunPathV1, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(w, r)

	if w.Code == http.StatusConflict {
		t.Fatalf("an agent enabled after the daemon started was refused with a 409: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "restart the daemon") {
		t.Fatalf("the refusal still sends the operator to a restart for an agent the runtime can now serve: %s", w.Body.String())
	}
	// The request has to get past the conflict for this test to mean anything:
	// a 4xx from somewhere else would leave the assertion above vacuous.
	if w.Code >= http.StatusBadRequest {
		t.Fatalf("status = %d (%s): the run never reached the agent it was addressed to", w.Code, w.Body.String())
	}
	t.Logf("status = %d", w.Code)
}
