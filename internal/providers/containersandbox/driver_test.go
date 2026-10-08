package containersandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

func testPolicy(engine, state string) middleware.ContainerSandbox {
	return middleware.ContainerSandbox{Engine: engine, Image: "ubuntu:24.04", WorkspaceAccess: "read-only", Network: "none", StateDir: state, User: "1000:1000", MemoryBytes: 64 * 1024 * 1024, CPUs: 1, Pids: 32}
}

func TestArgumentsKeepSecretsOutOfArgvAndDisableHostAuthority(t *testing.T) {
	launch := Launch{Policy: testPolicy("docker", "/state"), Command: "/bin/agent", Env: []string{"PROVIDER_TOKEN=PRIVATE_SENTINEL"}}
	args, env, err := createArguments(launch, boundaryPaths{"matrix-test", "sha256:cached", "/work space", "/state"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "PRIVATE_SENTINEL") || !strings.Contains(joined, "--pull never") || !strings.Contains(joined, "--read-only") || !strings.Contains(joined, "readonly") || !strings.Contains(joined, "--network none") {
		t.Fatal("unsafe launch arguments", joined)
	}
	if !strings.Contains(strings.Join(env, "\n"), "PROVIDER_TOKEN=PRIVATE_SENTINEL") {
		t.Fatal("explicit provider credential lost")
	}
	bounded := &boundedOutput{}
	_, _ = bounded.Write(make([]byte, 70000))
	if !bounded.overflow || len(bounded.data) != 65536 {
		t.Fatal("engine output not bounded")
	}
}

func TestRejectMissingEngineOverlappingMountsAndEnvironmentInjection(t *testing.T) {
	workspace := t.TempDir()
	if _, _, err := mountPaths(workspace, workspace); err == nil {
		t.Fatal("state overlaps workspace")
	}
	if _, _, err := mountPaths(workspace, filepath.Join(workspace, "missing")); err == nil {
		t.Fatal("missing mount accepted")
	}
	policy := testPolicy(filepath.Join(t.TempDir(), "missing-engine"), t.TempDir())
	if _, err := Start(context.Background(), Launch{Policy: policy, Workspace: workspace, Command: "/bin/agent"}); err == nil {
		t.Fatal("missing engine downgraded")
	}
	for _, env := range [][]string{{"DOCKER_HOST=tcp://other"}, {"HOME=/root"}, {"TOKEN=one", "TOKEN=two"}} {
		if _, _, err := createArguments(Launch{Policy: policy, Command: "agent", Env: env}, boundaryPaths{}); err == nil {
			t.Fatal("ambiguous environment accepted")
		}
	}
}

// Native real-engine proof is explicit: unit tests never substitute a fake
// successful mount/isolator for OS enforcement.
func TestSmokeRealContainerReadOnlyBoundaryAndCancelCleanup(t *testing.T) {
	engine := os.Getenv("MATRIX_REAL_CONTAINER_ENGINE")
	if engine == "" {
		t.Skip("set MATRIX_REAL_CONTAINER_ENGINE for a real cached-image proof")
	}
	workspace, state := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "marker"), []byte("VISIBLE"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	script := `test "$(cat /workspace/marker)" = VISIBLE && ! touch /workspace/denied && ! touch /root/denied && ! test -e /var/run/docker.sock && printf '{"boundary":"denied"}\n'; while read line; do printf '{"echo":"ok"}\n'; done`
	transport, err := Start(ctx, Launch{Policy: testPolicy(engine, state), Workspace: workspace, Command: "/bin/sh", Args: []string{"-c", script}})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	proof, err := transport.Receive(context.Background())
	if err != nil || string(proof) != `{"boundary":"denied"}` {
		t.Fatal("physical boundary unproven", string(proof), err)
	}
	cancel()
	select {
	case <-transport.done:
	case <-time.After(8 * time.Second):
		t.Fatal("cancel did not clean owned container")
	}
	if _, err := engineCommand(context.Background(), transport.engine, nil, "inspect", transport.name); err == nil {
		t.Fatal("owned container remains")
	}
	if _, err := os.Stat(filepath.Join(workspace, "denied")); !os.IsNotExist(err) {
		t.Fatal("workspace write escaped")
	}
}
