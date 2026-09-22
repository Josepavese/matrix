package agentcatalog

import (
	"context"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/middleware"
)

// TestRegisterRemoteRejectsIncompleteRegistrations keeps a half-specified agent
// out of the catalog: an entry with no id can never be addressed, and one with no
// protocol cannot be dialled, so both must be refused before persistence.
func TestRegisterRemoteRejectsIncompleteRegistrations(t *testing.T) {
	storage := memstore.New()
	cases := map[string]struct {
		storage middleware.Storage
		entry   Entry
		want    string
	}{
		"without storage": {nil, Entry{ID: "a", Kind: middleware.ProtocolKindA2A}, "storage not available"},
		"without id":      {storage, Entry{Kind: middleware.ProtocolKindA2A}, "agent ID is required"},
		"without kind":    {storage, Entry{ID: "a"}, "protocol kind is required"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := RegisterRemote(tc.storage, tc.entry); err == nil {
				t.Fatal("an incomplete registration must be refused")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q must explain the problem (%q)", err, tc.want)
			}
		})
	}
}

// TestRegisterRemoteDefaultsTheTransportPerProtocol pins the documented default:
// an A2A agent speaks JSONRPC and an ACP agent speaks ws unless the caller says
// otherwise, and the choice is persisted rather than re-derived later.
func TestRegisterRemoteDefaultsTheTransportPerProtocol(t *testing.T) {
	storage := memstore.New()
	if err := RegisterRemote(storage, Entry{ID: "a2a-agent", Kind: middleware.ProtocolKindA2A, Address: "https://peer.example"}); err != nil {
		t.Fatalf("register a2a: %v", err)
	}
	if err := RegisterRemote(storage, Entry{ID: "acp-agent", Kind: middleware.ProtocolKindACP, Address: "/usr/bin/codex"}); err != nil {
		t.Fatalf("register acp: %v", err)
	}
	if err := RegisterRemote(storage, Entry{ID: "explicit", Kind: middleware.ProtocolKindA2A, Transport: "grpc"}); err != nil {
		t.Fatalf("register explicit: %v", err)
	}

	cases := map[string]string{"a2a-agent": "JSONRPC", "acp-agent": "ws", "explicit": "grpc"}
	for id, want := range cases {
		entry, err := agentcfg.LoadEntry(storage, id)
		if err != nil {
			t.Fatalf("load %s: %v", id, err)
		}
		if entry.Config.Transport != want {
			t.Fatalf("agent %s transport = %q, want %q", id, entry.Config.Transport, want)
		}
	}
}

// TestResolveAndRegisterA2ARejectsAnUnresolvableCard keeps a registration whose
// card cannot be fetched from being silently accepted as a bare endpoint.
func TestResolveAndRegisterA2ARejectsAnUnresolvableCard(t *testing.T) {
	storage := memstore.New()
	_, err := ResolveAndRegisterA2A(context.Background(), storage, inertNetwork{}, A2ARegistration{
		ID:      "peer",
		CardURL: "https://peer.invalid/agent-card.json",
	})
	if err == nil {
		t.Fatal("an unresolvable card must be reported")
	}
}

// inertNetwork satisfies middleware.Network without I/O; validation must happen
// before any call, so this never has to answer.
type inertNetwork struct{}

func (inertNetwork) Listen(string, string) (middleware.ClosableListener, error) { return nil, nil }
func (inertNetwork) Download(context.Context, string, string) error             { return nil }
func (inertNetwork) FetchJSON(context.Context, string, interface{}) error       { return nil }
func (inertNetwork) GetFreePort() (int, error)                                  { return 0, nil }
func (inertNetwork) Fetch(context.Context, string) ([]byte, error)              { return nil, nil }
func (inertNetwork) PostJSON(context.Context, string, interface{}) ([]byte, int, error) {
	return nil, 0, nil
}
func (inertNetwork) CanDial(string) bool { return false }
