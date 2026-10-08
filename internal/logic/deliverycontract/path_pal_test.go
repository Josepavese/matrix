package deliverycontract

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceSymlinkAliasAcceptsOwnedArtifactAndRejectsEscape(t *testing.T) {
	physical := t.TempDir()
	alias := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(physical, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	write(t, physical, "owned.md", "DELIVERED")
	verdict := Evaluate(context.Background(), alias, Contract{Artifacts: []Artifact{{Path: "owned.md"}}})
	if verdict.Status != StatusAccepted {
		t.Fatal("canonical workspace alias misclassified as escape", verdict)
	}
	out := write(t, t.TempDir(), "outside.md", "OUTSIDE")
	if err := os.Symlink(out, filepath.Join(physical, "escaped.md")); err != nil {
		t.Fatal(err)
	}
	verdict = Evaluate(context.Background(), alias, Contract{Artifacts: []Artifact{{Path: "escaped.md"}}})
	if verdict.Status != StatusUnverifiable {
		t.Fatal("workspace alias weakened escape check")
	}
}

func TestPortableArtifactRejectsRootedAndDriveQualifiedPaths(t *testing.T) {
	for _, path := range []string{"/etc/passwd", `\windows\file`, `C:\file`, `C:relative`, "../outside", `..\outside`} {
		if _, err := resolveArtifact(t.TempDir(), path); err == nil {
			t.Fatal("nonportable rooted/escaped path accepted", path)
		}
	}
}
