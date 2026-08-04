//go:build linux || darwin

package agentinstall

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

func TestInstallCanonicalCodexActivatesCompleteStaging(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "node"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "npm"), "#!/bin/sh\nmkdir -p \"$3/node_modules/@agentclientprotocol/codex-acp/dist\"\ntouch \"$3/node_modules/@agentclientprotocol/codex-acp/dist/index.js\"\n")
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	target := filepath.Join(root, "codex")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "old"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := InstallCanonicalCodex(context.Background(), Config{
		FS: osfs.NewFSProvider(), Process: execprovider.NewProvider(), Target: target,
		Package: "@agentclientprotocol/codex-acp@1.1.2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Command != filepath.Join(bin, "node") || len(cfg.Args) != 1 || !strings.HasSuffix(cfg.Args[0], "dist/index.js") {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if got := strings.Join(cfg.Env, "\x00"); !strings.Contains(got, agentlaunch.CodexPolicyContractEnv+"="+agentlaunch.CodexPolicyContractV1) {
		t.Fatalf("missing Codex policy contract marker: %#v", cfg.Env)
	}
	if _, err := os.Stat(filepath.Join(target, "old")); !os.IsNotExist(err) {
		t.Fatalf("old install was not replaced: %v", err)
	}
}

func TestInstallCanonicalCodexPreservesExistingInstallOnNPMFailure(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "node"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "npm"), "#!/bin/sh\necho install-failed >&2\nexit 9\n")
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	target := filepath.Join(root, "codex")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "old")
	if err := os.WriteFile(marker, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := InstallCanonicalCodex(context.Background(), Config{
		FS: osfs.NewFSProvider(), Process: execprovider.NewProvider(), Target: target,
		Package: "@agentclientprotocol/codex-acp@1.1.2",
	})
	if err == nil || !strings.Contains(err.Error(), "exit code 9") {
		t.Fatalf("unexpected error: %v", err)
	}
	if content, readErr := os.ReadFile(marker); readErr != nil || string(content) != "old" {
		t.Fatalf("existing install changed: content=%q err=%v", content, readErr)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
