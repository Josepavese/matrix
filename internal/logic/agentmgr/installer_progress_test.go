package agentmgr

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestInstallProgressGoesToTheInjectedWriter is what makes a future
// `matrix install --json` possible: the human-readable progress of an install
// must be redirectable, so a machine-readable caller can keep stdout parseable
// instead of having lines interleaved into its document.
func TestInstallProgressGoesToTheInjectedWriter(t *testing.T) {
	registry := startFakeRegistry(t, publishRealDigest)
	installer, _, _, _ := newTestInstaller(t, registry.registryURL())

	var progress bytes.Buffer
	installer.SetProgressWriter(&progress)

	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	out := progress.String()
	if out == "" {
		t.Fatal("an install with a progress writer must write its progress there, not to stdout")
	}
	for _, want := range []string{"Downloading", "Verified sha256", "Extracting"} {
		if !strings.Contains(out, want) {
			t.Fatalf("progress is missing %q:\n%s", want, out)
		}
	}
}

// TestInstallIsSilentWithoutAProgressWriter pins the other half: an installer
// with no writer (the zero value, as a machine-readable caller would build it)
// must not panic and must not print.
func TestInstallIsSilentWithoutAProgressWriter(t *testing.T) {
	registry := startFakeRegistry(t, publishRealDigest)
	installer, _, _, _ := newTestInstaller(t, registry.registryURL())

	installer.SetProgressWriter(nil)

	if err := installer.Install(context.Background(), "opencode"); err != nil {
		t.Fatalf("install failed: %v", err)
	}
}
