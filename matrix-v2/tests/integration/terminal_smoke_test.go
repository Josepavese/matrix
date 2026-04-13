package integration

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
	"github.com/jose/matrix-v2/internal/providers/agents"
	exepprov "github.com/jose/matrix-v2/internal/providers/exec"
	"github.com/jose/matrix-v2/internal/providers/osfs"
)

// realAgentResolver resolves a real agent binary via a configurable protocol.
type realAgentResolver struct {
	protocol string
	bin      string
	args     []string
	env      []string
	wsAddr   string // used for ws protocol
}

func (r *realAgentResolver) GetAgentEndpoint(agentID string) (protocol, address string, args, env []string, err error) {
	if r.protocol == "ws" && r.wsAddr != "" {
		return "ws", r.wsAddr, nil, r.env, nil
	}
	return r.protocol, r.bin, r.args, r.env, nil
}

const smokeTestPrompt = "echo terminal-smoke-test-passed"
const smokeTestMarker = "terminal-smoke-test-passed"

func requireSmokeTest(t *testing.T) {
	t.Helper()
	if os.Getenv("MATRIX_SMOKE_TEST") == "" {
		t.Skip("Set MATRIX_SMOKE_TEST=1 to run real agent smoke tests.")
	}
}

func lookPath(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s binary not found in PATH", name)
	}
	return path
}

// getFreePort returns an available TCP port.
func getFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	_ = l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// TestSmoke_TerminalCreate_GeminiAgent tests terminal/create with Gemini via stdio ACP.
func TestSmoke_TerminalCreate_GeminiAgent(t *testing.T) {
	requireSmokeTest(t)
	geminiPath := lookPath(t, "gemini")
	tmpDir := t.TempDir()

	resolver := &realAgentResolver{
		protocol: "stdio",
		bin:      geminiPath,
		args:     []string{"--acp"},
	}
	router := agents.NewRouter(resolver)
	router.SetProcess(exepprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), tmpDir)
	defer router.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	output, _, _, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          "gemini",
		LogicalSessionID: "smoke-gemini",
		Message:          smokeTestPrompt,
	})
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	t.Logf("Gemini output: %s", output)
	// The agent may paraphrase the echo output. Check for either the literal
	// echo string or a clear success indicator from the agent's response.
	if !jsonContains(output, smokeTestMarker) && !jsonContains(output, "successful") {
		t.Errorf("output should contain %q or indicate success, got: %s", smokeTestMarker, output)
	}
}

// TestSmoke_TerminalCreate_OpenCodeAgent tests terminal/create with OpenCode via WebSocket ACP.
// NOTE: OpenCode ACP requires a TTY and dies when run without one.
// To test manually: start 'opencode acp --port 9999', then set resolver wsAddr to ws://127.0.0.1:9999.
func TestSmoke_TerminalCreate_OpenCodeAgent(t *testing.T) {
	requireSmokeTest(t)
	t.Skip("OpenCode ACP requires a TTY; cannot run in automated test mode.")
}

// TestSmoke_CwdCoherence_GeminiAgent verifies that the agent's terminal
// commands execute in the correct working directory, not the daemon's CWD.
func TestSmoke_CwdCoherence_GeminiAgent(t *testing.T) {
	requireSmokeTest(t)
	geminiPath := lookPath(t, "gemini")
	tmpDir := t.TempDir()

	// Create a unique marker file in tmpDir
	markerContent := "cwd-correct-xyzzy"
	markerFile := "cwd_test_marker.txt"
	if err := os.WriteFile(tmpDir+"/"+markerFile, []byte(markerContent), 0644); err != nil {
		t.Fatal(err)
	}

	resolver := &realAgentResolver{
		protocol: "stdio",
		bin:      geminiPath,
		args:     []string{"--acp"},
	}
	router := agents.NewRouter(resolver)
	router.SetProcess(exepprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), tmpDir)
	defer router.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	output, _, _, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          "gemini",
		LogicalSessionID: "smoke-cwd-gemini",
		Message:          fmt.Sprintf("cat %s", markerFile),
	})
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	t.Logf("Gemini cwd output: %s", output)
	if !jsonContains(output, markerContent) {
		t.Errorf("agent should read %q from %s — cwd is wrong. Output: %s", markerContent, markerFile, output)
	}
}
