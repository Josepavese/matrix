package daemon_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/daemon"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
	"github.com/Josepavese/matrix/internal/logic/vault"
	networkprovider "github.com/Josepavese/matrix/internal/providers/network"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/Josepavese/matrix/internal/providers/rpcstorage"
)

func TestRuntimeBrokerServesDaemonOwnedStorage(t *testing.T) {
	home := t.TempDir()
	fs := osfs.NewFSProvider()
	if err := fs.MkdirAll(filepath.Join(home, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := memstore.New()
	server := daemon.NewServer(vault.NewVault(store), networkprovider.NewProvider()).
		WithRuntimeBroker(store, fs, home, filepath.Join(home, "logs", "runtime.jsonl"))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Start(ctx, "127.0.0.1:0") }()

	descriptor := waitForBroker(t, fs, runtimebroker.Path(home))
	provider, err := rpcstorage.New(descriptor.JSONRPCAddr, descriptor.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	if err := provider.Set("agent.codex", []byte(`{"active":true}`)); err != nil {
		t.Fatal(err)
	}
	if value, err := provider.Get("agent.codex"); err != nil || string(value) != `{"active":true}` {
		t.Fatalf("get value=%q err=%v", value, err)
	}
	if keys, err := provider.List("agent."); err != nil || len(keys) != 1 || keys[0] != "agent.codex" {
		t.Fatalf("list keys=%v err=%v", keys, err)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(runtimebroker.Path(home)); !os.IsNotExist(err) {
		t.Fatalf("runtime descriptor retained after shutdown: %v", err)
	}
}

func waitForBroker(t *testing.T, fs *osfs.FSProvider, path string) runtimebroker.Descriptor {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if descriptor, err := runtimebroker.Read(fs, path); err == nil {
			return descriptor
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("runtime broker descriptor was not published")
	return runtimebroker.Descriptor{}
}
