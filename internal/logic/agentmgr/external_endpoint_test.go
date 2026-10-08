package agentmgr

import (
	"context"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/memstore"
)

func TestExternalACPEndpointDoesNotRequireMissingExecutable(t *testing.T) {
	for _, transport := range []string{"ws", "unix"} {
		t.Run(transport, func(t *testing.T) {
			store := memstore.New()
			cfg := agentcfg.Config{Kind: "acp", Transport: transport, Address: "/external/endpoint"}
			if err := agentcfg.SaveEntry(store, "external", agentcfg.Entry{Config: cfg}); err != nil {
				t.Fatal(err)
			}
			reg, err := NewRegistry(nil, store)
			if err != nil {
				t.Fatal(err)
			}
			proc := &startRecordingProcess{}
			s := NewSupervisor(proc, freePortNetwork{}, store, reg)
			if err := s.StartAll(context.Background()); err != nil {
				t.Fatal(err)
			}
			ep, err := s.GetAgentEndpoint("external")
			if err != nil || ep.Address != cfg.Address {
				t.Fatalf("remote endpoint was treated as missing child: %+v %v", ep, err)
			}
			if len(proc.specs) != 0 {
				t.Fatal("external peer was launched as a managed process")
			}
			if !isInstalledEndpoint(cfg, ep, proc) || runtimeMode(ep) != "external" {
				t.Fatal("doctor reports a missing local executable")
			}
		})
	}
}

func TestExternalEndpointRejectsManagedAndUnknownTransport(t *testing.T) {
	for _, cfg := range []AgentConfig{{Command: "agent", Kind: "acp", Transport: "ws", Address: "localhost"}, {Kind: "acp", Transport: "unknown", Address: "localhost"}} {
		if isRemoteACPEndpoint(cfg, protocolEndpointFromAgentConfig(cfg)) {
			t.Fatal("endpoint ownership or transport guessed")
		}
	}
}
