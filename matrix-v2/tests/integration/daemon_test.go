package integration

import (
	"net/rpc/jsonrpc"
	"path/filepath"
	"testing"
	"time"

	"github.com/jose/matrix-v2/internal/logic/daemon"
	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	networkprovider "github.com/jose/matrix-v2/internal/providers/network"
)

func TestDaemon_VaultRPC(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "integration_vault.db")

	provider, err := bolt.NewProvider(dbPath)
	if err != nil {
		t.Fatalf("Failed to init bolt provider: %v", err)
	}
	defer provider.Close()

	v := vault.NewVault(provider)
	netProvider := networkprovider.NewProvider()
	srv := daemon.NewServer(v, netProvider)

	// Start server on a specific local port
	addr := "127.0.0.1:9091"
	go func() {
		if err := srv.Start(addr); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait briefly for the server to start accepting connections
	time.Sleep(100 * time.Millisecond)
	defer srv.Stop()

	// Dial the JSON-RPC TCP server
	client, err := jsonrpc.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to dial TCP JSON-RPC server: %v", err)
	}
	defer client.Close()

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
