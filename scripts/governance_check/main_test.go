package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestAndCheck(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "doc.md"), "Matrix is protocol-neutral and channel-neutral.")
	mustWrite(t, filepath.Join(root, "ci.yml"), "go test ./...")

	manifestPath := filepath.Join(root, "manifest.toml")
	mustWrite(t, manifestPath, `
[documents]
required = ["doc.md"]

[required_text]
"doc.md" = ["protocol-neutral", "channel-neutral"]

[ci]
file = "ci.yml"
required = ["go test ./..."]
`)

	loaded, err := loadManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	report := checkManifest(root, loaded)
	if len(report.Failures) != 0 {
		t.Fatalf("unexpected failures: %#v", report.Failures)
	}
	if report.DocumentsChecked != 1 || report.TextContracts != 1 || report.FileGates != 1 {
		t.Fatalf("unexpected counts: %#v", report)
	}
}

func TestCheckManifestReportsMissingToken(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "doc.md"), "Matrix")

	report := checkManifest(root, manifest{
		Documents: []string{"doc.md"},
		RequiredText: map[string][]string{
			"doc.md": []string{"protocol-neutral"},
		},
	})

	if len(report.Failures) != 1 {
		t.Fatalf("expected one failure, got %#v", report.Failures)
	}
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
