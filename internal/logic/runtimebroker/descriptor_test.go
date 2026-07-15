package runtimebroker

import (
	"path/filepath"
	"runtime"
	"testing"

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
