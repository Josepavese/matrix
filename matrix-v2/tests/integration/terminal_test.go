package integration

import (
	"context"
	"os"
	goexec "os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
	"github.com/jose/matrix-v2/internal/providers/agents"
	exepprov "github.com/jose/matrix-v2/internal/providers/exec"
	"github.com/jose/matrix-v2/internal/providers/osfs"
)

// mockResolver resolves the mock-agent binary as a stdio endpoint.
type mockResolver struct {
	binPath string
}

func (m *mockResolver) GetAgentEndpoint(_ string) (protocol, address string, args, env []string, err error) {
	return "stdio", m.binPath, nil, nil, nil
}

func buildMockAgent(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	mockBin := filepath.Join(tmpDir, "mock-agent")

	buildCmd := goexec.Command("go", "build", "-o", mockBin, "github.com/jose/matrix-v2/cmd/mock-agent")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build mock-agent: %v\n%s", err, output)
	}
	return mockBin
}

func TestTerminalCreate_MockAgentIntegration(t *testing.T) {
	mockBin := buildMockAgent(t)
	tmpDir := t.TempDir()

	resolver := &mockResolver{binPath: mockBin}
	router := agents.NewRouter(resolver)
	router.SetProcess(exepprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), tmpDir)
	defer router.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, _, _, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          "test-agent",
		LogicalSessionID: "ch1",
		Message:          "__TERMINAL_TEST__",
	})
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	t.Logf("Agent output: %s", output)

	if !jsonContains(output, "exitCode") {
		t.Errorf("output should contain terminal result with exitCode, got: %s", output)
	}
	if !jsonContains(output, "from-mock-agent") {
		t.Errorf("output should contain echo output 'from-mock-agent', got: %s", output)
	}
}

func TestTerminalCreate_NormalThenTerminal(t *testing.T) {
	mockBin := buildMockAgent(t)
	tmpDir := t.TempDir()

	resolver := &mockResolver{binPath: mockBin}
	router := agents.NewRouter(resolver)
	router.SetProcess(exepprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), tmpDir)
	defer router.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First: normal prompt
	output, _, _, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          "test-agent",
		LogicalSessionID: "ch2",
		Message:          "hello",
	})
	if err != nil {
		t.Fatalf("normal prompt failed: %v", err)
	}
	if !jsonContains(output, "mock agent") {
		t.Errorf("normal prompt should contain 'mock agent', got: %s", output)
	}

	// Second: terminal test prompt (reuses cached client)
	output2, _, _, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          "test-agent",
		LogicalSessionID: "ch3",
		Message:          "__TERMINAL_TEST__",
	})
	if err != nil {
		t.Fatalf("terminal prompt failed: %v", err)
	}
	if !jsonContains(output2, "exitCode") {
		t.Errorf("terminal prompt should contain exitCode, got: %s", output2)
	}
}

func jsonContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
