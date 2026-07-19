package runtimevault_test

import (
	"bytes"
	"context"
	"encoding/base64"
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
		time.Sleep(100 * time.Millisecond)
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
	if err := runtimebroker.ClaimStartup(fs, runtimebroker.StartupPath(home), time.Now(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, "data", "matrix-vault.db")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startupErr := make(chan error, 1)
	serverDone := make(chan error, 1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		writer, err := bolt.NewProvider(dbPath)
		if err != nil {
			startupErr <- err
			return
		}
		defer writer.Close()
		if err := runtimebroker.RemoveStartup(fs, runtimebroker.StartupPath(home)); err != nil {
			startupErr <- err
			return
		}
		if err := writer.Set("probe", []byte(`"intent"`)); err != nil {
			startupErr <- err
			return
		}
		startupErr <- nil
		server := daemon.NewServer(vault.NewVault(writer), networkprovider.NewProvider()).
			WithRuntimeBroker(writer, fs, home, filepath.Join(home, "logs", "runtime.jsonl"))
		serverDone <- server.Start(ctx, "127.0.0.1:0")
	}()

	reader, err := runtimevault.OpenReadOnly(dbPath)
	if startupErr := <-startupErr; startupErr != nil {
		t.Fatalf("daemon startup: %v", startupErr)
	}
	if err != nil {
		t.Fatalf("CLI open during daemon startup: %v", err)
	}
	if value, err := reader.Get("probe"); err != nil || string(value) != `"intent"` {
		t.Fatalf("value=%q err=%v", value, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-serverDone; err != nil {
		t.Fatal(err)
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
