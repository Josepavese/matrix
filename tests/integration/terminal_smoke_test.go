package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runnotifier"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/agents"
	exepprov "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// realAgentResolver resolves a real agent binary via a configurable protocol.
type realAgentResolver struct {
	protocol string
	bin      string
	args     []string
	env      []string
	wsAddr   string // used for ws protocol
}

func (r *realAgentResolver) GetAgentEndpoint(_ string) (middleware.ProtocolEndpoint, error) {
	if r.protocol == "ws" && r.wsAddr != "" {
		return middleware.ProtocolEndpoint{
			Kind:      middleware.ProtocolKindACP,
			Transport: "ws",
			Address:   r.wsAddr,
			Env:       r.env,
		}, nil
	}
	return middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: r.protocol,
		Command:   r.bin,
		Args:      r.args,
		Env:       r.env,
	}, nil
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
	router.SetTrustMode(func() bool { return true })
	router.SetProcess(exepprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), tmpDir)
	defer router.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	output, _, _, _, err := router.Route(ctx, middleware.RouteRequest{
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

func TestSmoke_OpenCodeWS_ProjectsToolEvents(t *testing.T) {
	requireSmokeTest(t)
	wsAddr := os.Getenv("MATRIX_OPENCODE_ACP_WS")
	stdio := os.Getenv("MATRIX_OPENCODE_ACP_STDIO") == "1"
	if wsAddr == "" && !stdio {
		t.Skip("Set MATRIX_OPENCODE_ACP_WS=ws://127.0.0.1:<port> or MATRIX_OPENCODE_ACP_STDIO=1 to run OpenCode ACP smoke.")
	}
	tmpDir := os.Getenv("MATRIX_OPENCODE_ACP_WORKSPACE")
	if tmpDir == "" {
		tmpDir = t.TempDir()
	}
	writeFile(t, tmpDir+"/go.mod", "module checkoutsummary\n\ngo 1.23\n")
	writeFile(t, tmpDir+"/summary.go", "package checkoutsummary\n\nfunc Summary(items []string) string {\n\treturn \"todo\"\n}\n")
	writeFile(t, tmpDir+"/summary_test.go", "package checkoutsummary\n\nimport \"testing\"\n\nfunc TestSummary(t *testing.T) {\n\tgot := Summary([]string{\"apple\", \"pear\"})\n\twant := \"2 items: apple, pear\"\n\tif got != want {\n\t\tt.Fatalf(\"Summary() = %q, want %q\", got, want)\n\t}\n}\n")

	resolver := &realAgentResolver{
		protocol: "ws",
		wsAddr:   wsAddr,
	}
	if stdio {
		resolver.protocol = "stdio"
		resolver.bin = lookPath(t, "opencode")
		resolver.args = []string{"acp"}
	}
	router := agents.NewRouter(resolver)
	router.SetTrustMode(func() bool { return true })
	router.SetProcess(exepprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), tmpDir)
	defer router.Close()

	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{
		AgentID:     "opencode",
		Protocol:    "acp",
		ChannelID:   "smoke.opencode.tool-events",
		TracePolicy: runtrace.TracePolicy{ContentMode: runtrace.ContentModeInline, RedactionProfile: "frontend", IncludeProtocolMeta: false},
	})
	if err != nil {
		t.Fatalf("start run trace: %v", err)
	}
	notifier := runnotifier.New(store, run.ID, "opencode", "acp")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	output, _, _, _, err := router.Route(ctx, middleware.RouteRequest{
		AgentID:          "opencode",
		LogicalSessionID: "smoke-opencode-tools",
		WorkspacePath:    tmpDir,
		Message:          "Fix this Go package. Run go test ./... before final answer. Change only what is needed. Final answer must mention tests.",
		ThoughtNotifier:  notifier,
	})
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	t.Logf("OpenCode output: %s", output)

	cmd := exec.CommandContext(ctx, "go", "test", "./...")
	cmd.Dir = tmpDir
	testOutput, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test failed after OpenCode run: %v\n%s", err, string(testOutput))
	}

	trace, found, err := store.Trace(run.ID)
	if err != nil || !found {
		t.Fatalf("trace found=%v err=%v", found, err)
	}
	kinds := map[string]int{}
	toolKinds := map[string]int{}
	for _, event := range trace.Events {
		kinds[event.Kind]++
		if event.ToolKind != "" {
			toolKinds[event.ToolKind]++
		}
	}
	if kinds["tool.call.requested"] == 0 || kinds["tool.result.received"] == 0 {
		t.Fatalf("expected structural tool events, kinds=%v trace=%#v", kinds, trace.Events)
	}
	if toolKinds["edit"] == 0 && toolKinds["execute"] == 0 && toolKinds["read"] == 0 {
		t.Fatalf("expected structural tool kind, got %v", toolKinds)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(content, "\r\n", "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
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
	router.SetTrustMode(func() bool { return true })
	router.SetProcess(exepprov.NewProvider())
	router.SetFS(osfs.NewFSProvider(), tmpDir)
	defer router.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	output, _, _, _, err := router.Route(ctx, middleware.RouteRequest{
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
