package agentlaunch

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

// This probe reads only a disposable provider configuration, without credentials,
// model calls, user projects or the user's provider state. It proves loading the
// installed binary's permission schema, not physical sandbox enforcement.
func TestSmokeRealNativeProviderLoadsRestrictivePermissions(t *testing.T) {
	bin, contract := os.Getenv("MATRIX_REAL_NATIVE_BIN"), os.Getenv("MATRIX_REAL_NATIVE_CONTRACT")
	if bin == "" {
		t.Skip("set MATRIX_REAL_NATIVE_BIN and MATRIX_REAL_NATIVE_CONTRACT")
	}
	root := t.TempDir()
	endpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: bin, Env: []string{SandboxEnv + `={"native_contract":"` + contract + `","native_profile":"read-only"}`}}
	resolved, err := ResolveEndpoint("declared-contract", endpoint)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "debug", "config", "--pure")
	cmd.Dir = root
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "SystemRoot=" + os.Getenv("SystemRoot"), "USERPROFILE=" + root,
		"XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_DATA_HOME=" + filepath.Join(root, "data"), "XDG_STATE_HOME=" + filepath.Join(root, "state"), "XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "MIMOCODE_HOME=" + filepath.Join(root, "mimo"),
		"OPENCODE_DISABLE_AUTOUPDATE=1", "OPENCODE_DISABLE_MODELS_FETCH=1", "MIMOCODE_DISABLE_AUTOUPDATE=1", "MIMOCODE_DISABLE_MODELS_FETCH=1"}
	cmd.Env = append(cmd.Env, resolved.Endpoint.Env...)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("disposable native policy probe failed: %v", err)
	}
	var effective struct {
		Permission map[string]interface{} `json:"permission"`
	}
	if err := json.Unmarshal(output, &effective); err != nil {
		t.Fatal("provider configuration did not return its declared v1 schema")
	}
	for _, name := range []string{"external_directory", "bash", "edit", "task"} {
		if effective.Permission[name] != "deny" {
			t.Fatalf("native binary did not load denial for %s", name)
		}
	}
	t.Log("installed provider loaded read-only denials in disposable state; no model request")
}
