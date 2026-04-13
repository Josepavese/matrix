package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var binPath string

func TestMain(m *testing.M) {
	// Build the matrix binary for E2E testing
	tmpDir, err := os.MkdirTemp("", "matrix-e2e-*")
	if err != nil {
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binPath = filepath.Join(tmpDir, "matrix")
	cmd := exec.Command("go", "build", "-o", binPath, "github.com/jose/matrix-v2/cmd/matrix")
	if out, err := cmd.CombinedOutput(); err != nil {
		_, _ = os.Stderr.Write(out)
		return
	}

	// Run tests
	code := testscript.RunMain(m, map[string]func() int{})
	os.Exit(code)
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: ".",
		Setup: func(env *testscript.Env) error {
			// Add the temp directory containing the 'matrix' binary to PATH
			env.Setenv("PATH", filepath.Dir(binPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
			return nil
		},
	})
}
