package agentmgr

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/middleware"
)

// freezeRegistryTTL keeps a test's view of the vault from depending on the
// production window, and restores it for the next test.
func freezeRegistryTTL(t *testing.T, ttl time.Duration) {
	t.Helper()
	previous := registryTTL
	registryTTL = ttl
	t.Cleanup(func() { registryTTL = previous })
}

// registerAgent writes an agent definition the way `matrix agent enable` and the
// installer do: straight into the SSOT the running daemon reads.
func registerAgent(t *testing.T, store middleware.Storage, id, command string, active bool) {
	t.Helper()
	enabled := active
	if err := agentcfg.SaveEntry(store, id, agentcfg.Entry{
		Config: agentcfg.Config{Command: command, Kind: "acp", Transport: "stdio", Active: &enabled},
	}); err != nil {
		t.Fatalf("SaveEntry(%s): %v", id, err)
	}
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// reloadFailingStore is a vault whose listing can start failing, which is how a
// test tells "the agent is gone" apart from "the vault could not be read".
type reloadFailingStore struct {
	middleware.Storage
	fail bool
}

func (s *reloadFailingStore) List(prefix string) ([]string, error) {
	if s.fail {
		return nil, errors.New("vault unavailable")
	}
	return s.Storage.List(prefix)
}

// TestAnAgentEnabledAfterTheDaemonStartedIsServedNotRefusedAsUnknown is the
// acceptance test for the hot-enable window: the daemon's registry was built
// before the agent existed, and the agent must still be served. The TTL is set an
// hour away on purpose — what serves the agent is the reload a MISS triggers,
// not the clock, because a run can arrive a moment after the enable.
func TestAnAgentEnabledAfterTheDaemonStartedIsServedNotRefusedAsUnknown(t *testing.T) {
	store := memstore.New()
	registerAgent(t, store, "codex", "codex-acp", true)

	registry, err := NewRegistry(nil, store)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	freezeRegistryTTL(t, time.Hour)

	// The daemon is up and serving. The operator enables an agent now.
	registerAgent(t, store, "late-agent", "late-agent-acp", true)

	cfg, err := registry.Get("late-agent")
	if err != nil {
		t.Fatalf("an agent enabled after the daemon started was refused as unknown: %v", err)
	}
	if cfg.Command != "late-agent-acp" {
		t.Fatalf("command = %q, want the one just registered", cfg.Command)
	}
	if !containsID(registry.IDs(), "late-agent") {
		t.Fatal("the agent served by the miss is still absent from the registry view")
	}
	// An agent that really does not exist keeps its own answer.
	if _, err := registry.Get("agent-nobody-registered"); err == nil {
		t.Fatal("a lookup for an agent that was never registered succeeded")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("the refusal for an unknown agent lost its wording: %v", err)
	}
}

// TestAnEnabledAgentAppearsInTheViewWhenTheSnapshotExpires pins the other half of
// the freshness rule: inside the TTL the view is reused as it is, and past it the
// new agent is part of it — so a listing (which the supervisor walks at startup)
// cannot stay blind to the enable for longer than the window.
func TestAnEnabledAgentAppearsInTheViewWhenTheSnapshotExpires(t *testing.T) {
	store := memstore.New()
	registerAgent(t, store, "codex", "codex-acp", true)

	registry, err := NewRegistry(nil, store)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	freezeRegistryTTL(t, time.Hour)
	registerAgent(t, store, "late-agent", "late-agent-acp", true)

	if containsID(registry.List(), "late-agent") {
		t.Fatal("a snapshot inside its TTL was refreshed, so the window the TTL promises is not the one being tested")
	}

	registryTTL = 0
	if !containsID(registry.List(), "late-agent") {
		t.Fatal("an agent enabled before the snapshot expired is not in the view: the daemon would stay blind to it")
	}
}

// TestRegistryKeepsTheAgentsItHasWhenAReloadFails: a vault that cannot be read is
// not a reason to stop serving the agents already known, and it is not a reason
// to answer that an unknown agent does not exist either — the error says what
// happened.
func TestRegistryKeepsTheAgentsItHasWhenAReloadFails(t *testing.T) {
	store := &reloadFailingStore{Storage: memstore.New()}
	registerAgent(t, store, "codex", "codex-acp", true)

	registry, err := NewRegistry(nil, store)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	freezeRegistryTTL(t, 0)
	store.fail = true

	cfg, err := registry.Get("codex")
	if err != nil {
		t.Fatalf("a failing reload took away an agent the daemon was already serving: %v", err)
	}
	if cfg.Command != "codex-acp" {
		t.Fatalf("command = %q, want codex-acp", cfg.Command)
	}

	_, err = registry.Get("agent-never-registered")
	if err == nil {
		t.Fatal("a lookup succeeded while the vault was unreadable")
	}
	if strings.Contains(err.Error(), "not found") {
		t.Fatalf("an unreadable vault was reported as a missing agent: %v", err)
	}
}

// TestTheRunPathResolvesAnAgentEnabledAfterStartup drives the same freshness
// through the endpoint resolver the run API consults: a 409 said the agent was
// not served as ACP because this lookup came from a snapshot that predated it.
func TestTheRunPathResolvesAnAgentEnabledAfterStartup(t *testing.T) {
	store := memstore.New()
	registerAgent(t, store, "codex", "codex-acp", true)

	registry, err := NewRegistry(nil, store)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	supervisor := NewSupervisor(alwaysInstalledProcess{}, freePortNetwork{}, store, registry)
	freezeRegistryTTL(t, time.Hour)

	registerAgent(t, store, "late-agent", "late-agent-acp", true)

	endpoint, err := supervisor.GetAgentEndpoint("late-agent")
	if err != nil {
		t.Fatalf("the run path cannot resolve an agent enabled after startup: %v", err)
	}
	if endpoint.Kind != middleware.ProtocolKindACP || endpoint.Transport != "stdio" {
		t.Fatalf("endpoint = %s/%s, want acp/stdio: anything else is the 409 the operator saw",
			endpoint.Kind, endpoint.Transport)
	}
	if endpoint.Command != "late-agent-acp" {
		t.Fatalf("command = %q, want the one just registered", endpoint.Command)
	}
}
