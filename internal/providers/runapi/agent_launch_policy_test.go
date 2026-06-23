package runapi

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

type launchPolicyEndpointResolver struct {
	endpoint middleware.ProtocolEndpoint
	err      error
}

func (r launchPolicyEndpointResolver) GetAgentEndpoint(string) (middleware.ProtocolEndpoint, error) {
	if r.err != nil {
		return middleware.ProtocolEndpoint{}, r.err
	}
	return r.endpoint, nil
}

func TestAppendRouteEventsIncludesLaunchPolicyEvidence(t *testing.T) {
	store := memstore.New()
	server := NewServer(&runTestRouter{}).WithTraceStorage(store).WithEndpointResolver(launchPolicyEndpointResolver{
		endpoint: middleware.ProtocolEndpoint{
			Args: []string{"--dangerously-bypass-approvals-and-sandbox"},
		},
	})
	run, _, err := server.Store().Start(runtrace.Run{AgentID: "codex", Protocol: "acp", ChannelID: "test", InputRef: "matrix://pending/input"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	server.appendRouteEvents(run, "codex", "codex", nil)
	events, err := server.Store().LoadEvents(run.ID, 10)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	for _, event := range events {
		if event.Kind != "routing.decision" {
			continue
		}
		launchPolicy, ok := event.ProtocolMeta["agent_launch_policy"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected agent launch policy meta, got %+v", event.ProtocolMeta)
		}
		if launchPolicy["bypass_approvals_and_sandbox"] != true || launchPolicy["trusted_terminal"] != true {
			t.Fatalf("expected bypass trusted terminal evidence, got %+v", launchPolicy)
		}
		return
	}
	t.Fatalf("routing.decision event not found: %+v", events)
}
