package agents

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

func TestSandboxDisablesHostBridgesAndPermissionEscalation(t *testing.T) {
	endpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Env: []string{agentlaunch.SandboxEnv + `={"native_contract":"opencode-permission-v1","native_profile":"read-only"}`}}
	endpoint, deps, err := prepareSandboxClient(endpoint, middleware.ConversationFactoryDeps{FS: osfs.NewFSProvider(), Process: exec.NewProvider(), TrustMode: func() bool { return true }, TerminalAuth: true})
	if err != nil {
		t.Fatal(err)
	}
	if deps.FS != nil || deps.Process != nil || deps.TerminalAuth || deps.TrustMode() {
		t.Fatal("host authority escaped")
	}
	caps := acpClientCapabilitiesForDeps(deps)
	if caps.Fs.ReadTextFile || caps.Fs.WriteTextFile || caps.Terminal {
		t.Fatal("host capabilities falsely advertised")
	}
	handler := NewDefaultRequestHandler(deps.TrustMode).WithFS(deps.FS, t.TempDir())
	if _, err := handler.HandleRequest(context.Background(), "fs/write_text_file", json.RawMessage(`{"path":"/tmp/escape","content":"bad"}`)); err == nil {
		t.Fatal("unadvertised write bridge remains callable")
	}
	client := &acpConversationClient{endpoint: endpoint}
	if _, err := client.ExecuteTurn(context.Background(), middleware.ConversationTurn{Tools: []middleware.Tool{{Name: "host-tool"}}}); err == nil {
		t.Fatal("extension tool bypass accepted")
	}
}

func TestSandboxRejectsWorkspaceEscapeAndRetainsExactContainerMapping(t *testing.T) {
	root := t.TempDir()
	client := &acpConversationClient{cwd: root, endpoint: middleware.ProtocolEndpoint{Sandbox: &middleware.SandboxPolicy{Container: &middleware.ContainerSandbox{}}}}
	if mapped, err := client.sandboxWorkspace(root); err != nil || mapped != "/workspace" {
		t.Fatal("host path not mapped", mapped, err)
	}
	if _, err := client.sandboxWorkspace(t.TempDir()); err == nil {
		t.Fatal("other workspace accepted")
	}
	if _, err := client.additionalDirectories([]string{root}); err == nil {
		t.Fatal("extra directory bypass")
	}
	if err := client.verifySandboxSessionCwd("/other", "/workspace"); err == nil {
		t.Fatal("wrong provider workspace accepted")
	}
	if err := client.verifySandboxSessionCwd("/workspace", "/workspace"); err != nil {
		t.Fatal("guest mapping incorrectly checked on host", err)
	}
}

func TestCachedLaunchRejectsChangedSandboxBeforeReuse(t *testing.T) {
	endpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Env: []string{agentlaunch.SandboxEnv + `={"native_contract":"mimocode-permission-v1","native_profile":"workspace"}`}}
	resolved, err := agentlaunch.ResolveEndpoint("arbitrary", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	client := &acpConversationClient{endpoint: resolved.Endpoint}
	router := NewRouter(codexPolicyResolver{endpoint: endpoint})
	defer router.Close()
	key := clientCacheKey("arbitrary", t.TempDir())
	if matches, err := router.cachedLaunchMatches(key, client); err != nil || !matches {
		t.Fatal("identical policy not reusable", err)
	}
	endpoint.Env[0] = agentlaunch.SandboxEnv + `={"native_contract":"mimocode-permission-v1","native_profile":"read-only"}`
	router.resolver = codexPolicyResolver{endpoint: endpoint}
	if matches, err := router.cachedLaunchMatches(key, client); err != nil || matches {
		t.Fatal("more permissive cached process reused", err)
	}
}
