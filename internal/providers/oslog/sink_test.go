package oslog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestFactoryBuildFileSinkWithRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.jsonl")

	factory := NewFactory()
	sink, err := factory.Build(middleware.LogSinkOptions{
		Target:     "file",
		FilePath:   path,
		MaxBytes:   40,
		MaxBackups: 2,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	defer func() { _ = sink.Close() }()

	payload := strings.Repeat("x", 32) + "\n"
	if _, err := sink.Writer().Write([]byte(payload)); err != nil {
		t.Fatalf("first write error = %v", err)
	}
	if _, err := sink.Writer().Write([]byte(payload)); err != nil {
		t.Fatalf("second write error = %v", err)
	}

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected rotated backup file, got error: %v", err)
	}
}

func TestFactoryBuildFileSinkRestrictsPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.jsonl")

	sink, err := NewFactory().Build(middleware.LogSinkOptions{
		Target:     "file",
		FilePath:   path,
		MaxBytes:   1024,
		MaxBackups: 2,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	defer func() { _ = sink.Close() }()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("log file permissions = %o, want 600", got)
	}
}

func TestFactoryBuildBothSink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.jsonl")

	factory := NewFactory()
	sink, err := factory.Build(middleware.LogSinkOptions{
		Target:     "both",
		FilePath:   path,
		MaxBytes:   1024,
		MaxBackups: 2,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	defer func() { _ = sink.Close() }()

	if sink.Descriptor() == "" {
		t.Fatal("expected non-empty descriptor")
	}
}
