package runtimebroker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/providers/osfs"
)

func validDescriptor() Descriptor {
	return Descriptor{Version: Version, JSONRPCAddr: "127.0.0.1:5555", Token: "token"}
}

// TestReadRejectsInvalidDescriptors is the trust boundary for the broker file:
// anything that does not carry a usable address and token must be refused, or a
// stale or forged descriptor would point clients at the wrong runtime.
func TestReadRejectsInvalidDescriptors(t *testing.T) {
	fs := osfs.NewFSProvider()
	cases := map[string]string{
		"missing file":  "",
		"not json":      "{",
		"empty object":  "{}",
		"wrong version": `{"version":99,"jsonrpc_addr":"127.0.0.1:1","token":"t"}`,
		"missing addr":  `{"version":1,"token":"t"}`,
		"missing token": `{"version":1,"jsonrpc_addr":"127.0.0.1:1"}`,
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runtime-broker.json")
			if contents != "" {
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Read(fs, path); err == nil {
				t.Fatalf("a %s descriptor must be rejected", name)
			}
		})
	}
}

func TestWriteThenReadRoundTripsAndIsPrivate(t *testing.T) {
	fs := osfs.NewFSProvider()
	home := t.TempDir()
	path := Path(home)
	// The caller owns the home layout; Write only owns the file.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := Write(fs, path, validDescriptor()); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// The token authorises vault access: the descriptor must not be world readable.
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("descriptor mode is %v, want 0600", info.Mode().Perm())
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("the temporary file must not survive the write")
	}

	read, err := Read(fs, path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if read != validDescriptor() {
		t.Fatalf("round trip changed the descriptor: %+v", read)
	}
	if err := Remove(fs, path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := Read(fs, path); err == nil {
		t.Fatal("a removed descriptor must no longer be readable")
	}
}

// TestWriteFailsWithoutRemovingThePreviousDescriptor matters at runtime: a write
// that cannot complete must not leave the broker file missing, or a restarting
// client would conclude there is no runtime.
func TestWriteFailsWhenTheDirectoryIsNotWritable(t *testing.T) {
	fs := osfs.NewFSProvider()
	path := filepath.Join(t.TempDir(), "absent", "runtime-broker.json")
	err := Write(fs, path, validDescriptor())
	if err == nil {
		t.Fatal("writing into a missing directory must fail")
	}
	if !strings.Contains(err.Error(), "runtime broker descriptor") {
		t.Fatalf("the error must say what failed, got %q", err)
	}
}
