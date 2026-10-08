package containersandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSmokeRealValidatorBoundaryExitAndPersistentState(t *testing.T) {
	engine := os.Getenv("MATRIX_REAL_CONTAINER_ENGINE")
	if engine == "" {
		t.Skip("set MATRIX_REAL_CONTAINER_ENGINE for a real cached-image validator")
	}
	workspace, state := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "marker"), []byte("VISIBLE"), 0644); err != nil {
		t.Fatal(err)
	}
	policy := testPolicy(engine, state)
	launch := Launch{Policy: policy, Workspace: workspace, Identity: "validator-proof", Command: "/bin/sh", Args: []string{"-c", `test "$(cat /workspace/marker)" = VISIBLE && ! touch /workspace/escape && printf SAVED > "$HOME/session-marker"`}}
	if code, err := Exec(context.Background(), launch); err != nil || code != 0 {
		t.Fatal("validator boundary failed", code, err)
	}
	launch.Args = []string{"-c", `test "$(cat "$HOME/session-marker")" = SAVED || exit 9; exit 7`}
	if code, err := Exec(context.Background(), launch); err != nil || code != 7 {
		t.Fatal("real exit or persistent state lost", code, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "escape")); !os.IsNotExist(err) {
		t.Fatal("validator write escaped")
	}
	first := StateDirectory(state, workspace, "one", launch.Command)
	second := StateDirectory(state, workspace, "two", launch.Command)
	if first == second {
		t.Fatal("agent states overlap")
	}
}
