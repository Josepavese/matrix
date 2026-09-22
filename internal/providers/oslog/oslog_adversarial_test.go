package oslog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestBuildRejectsAnUnknownTarget keeps a typo in configuration from silently
// falling back to a sink the operator did not ask for: a lost log destination is
// discovered only when the logs are needed.
func TestBuildRejectsAnUnknownTarget(t *testing.T) {
	factory := NewFactory()
	for _, target := range []string{"", "stdout", "FILE", "syslog "} {
		sink, err := factory.Build(middleware.LogSinkOptions{Target: target})
		if err == nil {
			_ = sink.Close()
			t.Fatalf("target %q must be refused", target)
		}
		if !strings.Contains(err.Error(), "unsupported log sink target") {
			t.Fatalf("the error must name the problem, got %q", err)
		}
	}
}

// TestBuildRequiresAUsablePathForFileSinks keeps a file sink from being created
// at an unusable location, where writes would fail at the first log line.
func TestBuildRequiresAUsablePathForFileSinks(t *testing.T) {
	factory := NewFactory()
	if _, err := factory.Build(middleware.LogSinkOptions{Target: "file", FilePath: ""}); err == nil {
		t.Fatal("a file sink without a path must be refused")
	}
	// A missing parent directory is created on purpose, and the resulting file
	// must not be readable by other users: it carries agent output.
	path := filepath.Join(t.TempDir(), "missing-dir", "matrix.log")
	sink, err := factory.Build(middleware.LogSinkOptions{Target: "file", FilePath: path})
	if err != nil {
		t.Fatalf("the sink must create its directory: %v", err)
	}
	defer func() { _ = sink.Close() }()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("log file mode is %v, want 0600", info.Mode().Perm())
	}
	// An unusable location must still be refused rather than silently dropped.
	if _, err := factory.Build(middleware.LogSinkOptions{Target: "file", FilePath: "/proc/denied/matrix.log"}); err == nil {
		t.Fatal("an unusable log path must be refused")
	}
}

func TestBuildAcceptsTheDocumentedTargets(t *testing.T) {
	factory := NewFactory()
	stderr, err := factory.Build(middleware.LogSinkOptions{Target: "stderr"})
	if err != nil {
		t.Fatalf("stderr sink: %v", err)
	}
	_ = stderr.Close()

	path := filepath.Join(t.TempDir(), "matrix.log")
	file, err := factory.Build(middleware.LogSinkOptions{Target: "file", FilePath: path, MaxBytes: 1024, MaxBackups: 1})
	if err != nil {
		t.Fatalf("file sink: %v", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Writer().Write([]byte("line\n")); err != nil {
		t.Fatalf("writing to the file sink: %v", err)
	}
}
