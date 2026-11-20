package runtimebroker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/providers/osfs"
)

func TestDescriptorRoundTripUsesPrivateRuntimeToken(t *testing.T) {
	home := t.TempDir()
	path := Path(home)
	if err := osfs.NewFSProvider().MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	descriptor, err := New("127.0.0.1:9090", "logs/runtime.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(osfs.NewFSProvider(), path, descriptor); err != nil {
		t.Fatal(err)
	}
	got, err := Read(osfs.NewFSProvider(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != descriptor.Token || len(got.Token) != 64 || !filepath.IsAbs(got.LogFile) {
		t.Fatalf("unexpected descriptor: %+v", got)
	}
	if info, err := osfs.NewFSProvider().Stat(path); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("descriptor mode = %o", info.Mode().Perm())
	}
	if err := Remove(osfs.NewFSProvider(), path); err != nil {
		t.Fatal(err)
	}
}

func TestStartupMarkerRoundTripAndRemoval(t *testing.T) {
	home := t.TempDir()
	path := StartupPath(home)
	fs := osfs.NewFSProvider()
	if err := fs.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := WriteStartup(fs, path, now); err != nil {
		t.Fatal(err)
	}
	raw, err := fs.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var startup Startup
	if err := json.Unmarshal(raw, &startup); err != nil {
		t.Fatal(err)
	}
	if startup.State != "initializing" || startup.PID != os.Getpid() || !startup.StartedAt.Equal(now) {
		t.Fatalf("unexpected startup state: %+v", startup)
	}
	if err := Remove(fs, path); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("startup marker remains: %v", err)
	}
}
