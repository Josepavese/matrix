package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
)

func nonsandboxedLaunch(t *testing.T, env ...string) (middleware.ProtocolEndpoint, middleware.ProtocolEndpoint) {
	t.Helper()
	endpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: "/opt/opencode", Env: env}
	resolved, err := agentlaunch.ResolveEndpoint("arbitrary", endpoint)
	if err != nil {
		t.Fatalf("resolve endpoint: %v", err)
	}
	return endpoint, resolved.Endpoint
}

// TestCachedLaunchRejectsChangedEnvironmentWithoutSandbox is the regression for
// the reported gap: the historical early return let a nonsandboxed cached child
// survive a changed launch environment, so the updated configuration was
// presented while the running child still had the old one.
func TestCachedLaunchRejectsChangedEnvironmentWithoutSandbox(t *testing.T) {
	endpoint, cached := nonsandboxedLaunch(t, "OPENCODE_ENABLE_EXA=0")
	client := &acpConversationClient{endpoint: cached}
	router := NewRouter(codexPolicyResolver{endpoint: endpoint})
	defer router.Close()
	key := clientCacheKey("arbitrary", t.TempDir())
	if matches, err := router.cachedLaunchMatches(key, client); err != nil || !matches {
		t.Fatalf("unchanged launch declaration must be reusable: matches=%v err=%v", matches, err)
	}
	endpoint.Env = []string{"OPENCODE_ENABLE_EXA=1"}
	router.resolver = codexPolicyResolver{endpoint: endpoint}
	if matches, err := router.cachedLaunchMatches(key, client); err != nil || matches {
		t.Fatalf("a changed launch environment must not reuse the old child: matches=%v err=%v", matches, err)
	}
}

// TestChangedLaunchEnvironmentFailsClosedWithoutTouchingTheLease pins the
// preservation half of the contract: detecting the change refuses the new
// dispatch and leaves the in-flight client to its own reap/relaunch workflow.
func TestChangedLaunchEnvironmentFailsClosedWithoutTouchingTheLease(t *testing.T) {
	endpoint, cached := nonsandboxedLaunch(t, "OPENCODE_ENABLE_EXA=0")
	ctx := context.Background()
	client := &acpConversationClient{endpoint: cached, client: newBlockingACPClient(ctx)}
	client.activeClientLeases = 1 // a session turn is in flight
	router := NewRouter(codexPolicyResolver{endpoint: endpoint})
	defer router.Close()
	cwd := t.TempDir()
	key := clientCacheKey("arbitrary", cwd)
	router.clients[key] = client

	endpoint.Env = []string{"OPENCODE_ENABLE_EXA=1"}
	router.resolver = codexPolicyResolver{endpoint: endpoint}
	if _, err := router.getOrCreateClientLocked(ctx, "arbitrary", cwd); err == nil ||
		!strings.Contains(err.Error(), "launch declaration changed") {
		t.Fatalf("changed environment must fail closed with the reap workflow, got %v", err)
	}
	if router.clients[key] != client {
		t.Fatal("a leased client must not be evicted by the mismatch check")
	}
	if !client.Alive() {
		t.Fatal("a leased client must not be closed by the mismatch check")
	}
}

// TestUnchangedLaunchEnvironmentReplaysTheCachedClient covers the replay the
// report asks to qualify: with the declaration unchanged, a cached client is
// reused rather than reaped and relaunched.
func TestUnchangedLaunchEnvironmentReplaysTheCachedClient(t *testing.T) {
	endpoint, cached := nonsandboxedLaunch(t, "OPENCODE_ENABLE_EXA=0")
	ctx := context.Background()
	client := &acpConversationClient{endpoint: cached, client: newBlockingACPClient(ctx)}
	router := NewRouter(codexPolicyResolver{endpoint: endpoint})
	defer router.Close()
	cwd := t.TempDir()
	key := clientCacheKey("arbitrary", cwd)
	router.clients[key] = client

	reused, err := router.getOrCreateClientLocked(ctx, "arbitrary", cwd)
	if err != nil {
		t.Fatalf("unchanged launch declaration must replay: %v", err)
	}
	if reused != client {
		t.Fatal("unchanged launch declaration must reuse the cached client")
	}
}
