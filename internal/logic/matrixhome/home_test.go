package matrixhome

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveUsesExplicitMatrixHome(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom-home")
	t.Setenv(EnvName, want)

	got, err := Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveIgnoresRepositoryCwdWithoutExplicitHome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("default path assertion is linux-specific")
	}
	root := t.TempDir()
	xdg := filepath.Join(t.TempDir(), "xdg")
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "configs", "agents.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write agents.json: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	t.Setenv(EnvName, "")
	t.Setenv("XDG_DATA_HOME", xdg)

	got, err := Resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := filepath.Join(xdg, "matrix")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestEnsureCreatesPALHomeLayout(t *testing.T) {
	home := t.TempDir()
	if err := Ensure(home); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	for _, dir := range []string{"bin", "configs", "data", "logs", "artifacts", "agents", "backups", "tmp"} {
		if st, err := os.Stat(filepath.Join(home, dir)); err != nil || !st.IsDir() {
			t.Fatalf("expected directory %s, stat=%v err=%v", dir, st, err)
		}
	}
}

func TestAgentsDirUsesPALHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "matrix-home")
	if got, want := AgentsDir(home), filepath.Join(home, "agents"); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
