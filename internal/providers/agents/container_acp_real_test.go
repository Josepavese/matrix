package agents

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
)

func TestSmokeRealContainerACPStrictRestoreAndBridgeDenial(t *testing.T) {
	engine := os.Getenv("MATRIX_REAL_CONTAINER_ENGINE")
	if engine == "" || runtime.GOOS != "linux" {
		t.Skip("real Linux engine/cached image required for this fixture binary")
	}
	workspace, state := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(workspace, "peer"), ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("fixture build failed: %v %s", err, output)
	}
	t.Setenv("MATRIX_CONTAINER_HOST_SECRET", "HOST_ONLY_SENTINEL")
	policy := middleware.SandboxPolicy{Container: &middleware.ContainerSandbox{Engine: engine, Image: "ubuntu:24.04", WorkspaceAccess: "read-only", Network: "none", StateDir: state, User: "1000:1000", MemoryBytes: 128 << 20, CPUs: 1, Pids: 128}}
	declaration, _ := json.Marshal(policy)
	endpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: "/workspace/peer", Args: []string{"-test.run=^TestContainerACPFixturePeer$"}, Env: []string{agentlaunch.SandboxEnv + "=" + string(declaration), "MATRIX_CONTAINER_ACP_PEER=1"}}
	deps := middleware.ConversationFactoryDeps{AgentID: "container-fixture", Cwd: workspace}
	first, err := (&acpConversationFactory{}).NewClient(ctx, endpoint, deps)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	created, err := first.ExecuteTurn(ctx, middleware.ConversationTurn{AgentID: deps.AgentID, WorkspacePath: workspace, Message: "fixture proof"})
	if err != nil || created.RemoteSessionID != "container-proof-session" || !strings.Contains(created.Output, `"loaded":false`) {
		t.Fatal("container ACP creation failed", created.Output, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := (&acpConversationFactory{}).NewClient(ctx, endpoint, deps)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	restored, err := second.ExecuteTurn(ctx, middleware.ConversationTurn{AgentID: deps.AgentID, WorkspacePath: workspace, RemoteSessionID: created.RemoteSessionID, StrictSession: true, Message: "fixture restore proof"})
	if err != nil || restored.RemoteSessionID != created.RemoteSessionID || !strings.Contains(restored.Output, `"loaded":true`) {
		t.Fatal("exact session not restored", restored.Output, err)
	}
	if !strings.Contains(restored.Output, `"workspace_write":"denied"`) {
		t.Fatal("physical write boundary unproven", restored.Output)
	}
	evidence, ok := restored.Metadata.Meta["sandbox_execution"].(middleware.SandboxExecutionEvidence)
	if !ok || evidence.Verification != "engine_configuration_verified" || !strings.HasPrefix(evidence.ImageID, "sha256:") {
		t.Fatal("actual driver evidence missing")
	}
}

func TestContainerACPFixturePeer(_ *testing.T) {
	if os.Getenv("MATRIX_CONTAINER_ACP_PEER") != "1" {
		return
	}
	loaded := false
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req codexPolicyRPCRequest
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(3)
		}
		result := json.RawMessage(`{}`)
		switch req.Method {
		case "initialize":
			var init struct {
				ClientCapabilities struct {
					Fs       struct{ ReadTextFile, WriteTextFile bool }
					Terminal bool
				}
			}
			_ = json.Unmarshal(req.Params, &init)
			if init.ClientCapabilities.Fs.ReadTextFile || init.ClientCapabilities.Fs.WriteTextFile || init.ClientCapabilities.Terminal || os.Getenv("MATRIX_CONTAINER_HOST_SECRET") != "" {
				os.Exit(4)
			}
			result = json.RawMessage(`{"protocolVersion":1,"agentCapabilities":{"loadSession":true}}`)
		case "session/new", "session/load":
			loaded = containerFixtureSession(req)
			if req.Method == "session/new" {
				result = json.RawMessage(`{"sessionId":"container-proof-session"}`)
			}
		case "session/prompt":
			if err := os.WriteFile("/workspace/escape", []byte("BAD"), 0600); err == nil {
				os.Exit(5)
			}
			proof, _ := json.Marshal(map[string]interface{}{"loaded": loaded, "workspace_write": "denied"})
			writeCodexPolicyNotification("container-proof-session", string(proof))
			result = json.RawMessage(`{"stopReason":"end_turn"}`)
		}
		writeCodexPolicyRPC(codexPolicyRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
	}
	os.Exit(0)
}

func containerFixtureSession(req codexPolicyRPCRequest) bool {
	var session struct {
		Cwd       string
		SessionID string
	}
	if json.Unmarshal(req.Params, &session) != nil || session.Cwd != "/workspace" {
		os.Exit(6)
	}
	marker := filepath.Join(os.Getenv("HOME"), "exact-session")
	if req.Method == "session/new" {
		if _, err := os.Stat(marker); err == nil {
			os.Exit(7)
		}
		if os.WriteFile(marker, []byte("container-proof-session"), 0600) != nil {
			os.Exit(8)
		}
		return false
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != session.SessionID || session.SessionID != "container-proof-session" {
		os.Exit(9)
	}
	return true
}
