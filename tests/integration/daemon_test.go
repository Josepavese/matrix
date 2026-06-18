package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/rpc/jsonrpc"
	"path/filepath"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/daemon"
	"github.com/Josepavese/matrix/internal/logic/vault"
	"github.com/Josepavese/matrix/internal/providers/bolt"
	networkprovider "github.com/Josepavese/matrix/internal/providers/network"
)

func TestDaemon_VaultRPC(t *testing.T) {
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{5}, 32)))
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "integration_vault.db")

	provider, err := bolt.NewProvider(dbPath)
	if err != nil {
		t.Fatalf("Failed to init bolt provider: %v", err)
	}
	defer func() { _ = provider.Close() }()

	v := vault.NewVault(provider)
	netProvider := networkprovider.NewProvider()
	srv := daemon.NewServer(v, netProvider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := srv.Start(ctx, "127.0.0.1:0"); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	defer func() {
		if err := srv.Stop(); err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
	}()

	addr := waitForDaemonAddr(t, srv)
	client, err := jsonrpc.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to dial TCP JSON-RPC server: %v", err)
	}
	defer func() { _ = client.Close() }()

	key := "integration.test.key"
	expectedVal := "rpc_secret_value"

	// Test 1: Vault.Set
	setArgs := &daemon.VaultArgs{Key: key, Value: expectedVal}
	var setReply daemon.VaultReply
	err = client.Call("Vault.Set", setArgs, &setReply)
	if err != nil {
		t.Fatalf("RPC Call Vault.Set failed: %v", err)
	}

	// Test 2: Vault.Get
	getArgs := &daemon.VaultArgs{Key: key}
	var getReply daemon.VaultReply
	err = client.Call("Vault.Get", getArgs, &getReply)
	if err != nil {
		t.Fatalf("RPC Call Vault.Get failed: %v", err)
	}

	if getReply.Value != expectedVal {
		t.Errorf("RPC Vault.Get returned %s, expected %s", getReply.Value, expectedVal)
	}
}

func waitForDaemonAddr(t *testing.T, srv *daemon.Server) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if addr := srv.Addr(); addr != "" {
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("daemon did not expose a listener address")
	return ""
}
