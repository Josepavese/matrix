package runtimevault_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/daemon"
	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
	"github.com/Josepavese/matrix/internal/logic/vault"
	"github.com/Josepavese/matrix/internal/providers/bolt"
	networkprovider "github.com/Josepavese/matrix/internal/providers/network"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/Josepavese/matrix/internal/providers/runtimevault"
)

func TestOpenUsesBrokerWhileDaemonOwnsBolt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	fs := osfs.NewFSProvider()
	if err := fs.MkdirAll(filepath.Join(home, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, "data", "matrix-vault.db")
	writer, err := bolt.NewProvider(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := writer.Set("probe", []byte(`"ok"`)); err != nil {
		t.Fatal(err)
	}

	server := daemon.NewServer(vault.NewVault(writer), networkprovider.NewProvider()).
		WithRuntimeBroker(writer, fs, home, filepath.Join(home, "logs", "runtime.jsonl"))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Start(ctx, "127.0.0.1:0") }()
	waitForDescriptor(t, fs, runtimebroker.Path(home))

	started := time.Now()
	reader, err := runtimevault.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("broker read opened too slowly: %s", elapsed)
	}
	if value, err := reader.Get("probe"); err != nil || string(value) != `"ok"` {
		t.Fatalf("value=%q err=%v", value, err)
	}
	encrypted, plaintext, err := reader.InspectRawEncryption()
	if err != nil {
		t.Fatal(err)
	}
	if encrypted != 1 || plaintext != 0 {
		t.Fatalf("encrypted=%d plaintext=%d", encrypted, plaintext)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestOpenRetriesBrokerPublishedDuringBoltLockWait(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)))
	fs := osfs.NewFSProvider()
	if err := fs.MkdirAll(filepath.Join(home, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, "data", "matrix-vault.db")
	writer, err := bolt.NewProvider(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := writer.Set("probe", []byte(`"startup"`)); err != nil {
		t.Fatal(err)
	}

	server := daemon.NewServer(vault.NewVault(writer), networkprovider.NewProvider()).
		WithRuntimeBroker(writer, fs, home, filepath.Join(home, "logs", "runtime.jsonl"))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		// No startup delay: the ordering this test needs comes from the broker
		// descriptor and the bolt lock, so the daemon is free to start
		// concurrently with the read-only open — which is the interleaving under
		// test anyway.
		done <- server.Start(ctx, "127.0.0.1:0")
	}()

	reader, err := runtimevault.OpenReadOnly(dbPath)
	if err != nil {
		cancel()
		t.Fatalf("open during broker startup: %v", err)
	}
	if value, err := reader.Get("probe"); err != nil || string(value) != `"startup"` {
		t.Fatalf("value=%q err=%v", value, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestOpenWaitsForBrokerWhileDaemonStartupOwnsVaultIntent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)))
	fs := osfs.NewFSProvider()
	if err := fs.MkdirAll(filepath.Join(home, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, "data", "matrix-vault.db")
	writer, err := runtimevault.OpenDaemon(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	marker := runtimebroker.StartupPath(home)
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(marker, old, old); err != nil {
		t.Fatal(err)
	}
	if duplicate, err := runtimevault.OpenDaemon(dbPath); err == nil {
		_ = duplicate.Close()
		t.Fatal("a second daemon claimed the live runtime")
	}
	if reader, err := runtimevault.OpenReadOnly(dbPath); !errors.Is(err, runtimevault.ErrBrokerStarting) {
		if reader != nil {
			_ = reader.Close()
		}
		t.Fatalf("open before broker publication: %v", err)
	}
	if err := writer.Set("probe", []byte(`"intent"`)); err != nil {
		t.Fatal(err)
	}
	server := daemon.NewServer(vault.NewVault(writer), networkprovider.NewProvider()).
		WithRuntimeBroker(writer, fs, home, filepath.Join(home, "logs", "runtime.jsonl"))
	ctx, cancel := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Start(ctx, "127.0.0.1:0") }()
	t.Cleanup(func() {
		cancel()
		if err := <-serverDone; err != nil {
			t.Error(err)
		}
	})

	reader, err := runtimevault.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("CLI open during daemon startup: %v", err)
	}
	if value, err := reader.Get("probe"); err != nil || string(value) != `"intent"` {
		t.Fatalf("value=%q err=%v", value, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("startup marker remains after broker published: %v", err)
	}
}

func TestOpenIgnoresStartupMarkerWithoutAnOwner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)
	fs := osfs.NewFSProvider()
	marker := runtimebroker.StartupPath(home)
	if err := fs.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runtimebroker.WriteStartup(fs, marker, time.Now()); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, "data", "matrix-vault.db")
	vault, err := runtimevault.Open(dbPath)
	if err != nil {
		t.Fatalf("stale startup marker blocked offline access: %v", err)
	}
	if err := vault.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err := runtimevault.OpenDaemon(dbPath)
	if err != nil {
		t.Fatalf("stale startup marker prevented restart: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("startup marker remains after close: %v", err)
	}
}

func waitForDescriptor(t *testing.T, fs *osfs.FSProvider, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := runtimebroker.Read(fs, path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("runtime descriptor was not published")
}
