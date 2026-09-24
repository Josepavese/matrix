//go:build linux || darwin

package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/providers/matrixapi"
)

func TestLocalNotificationSocketIsPrivateAndServesCursor(t *testing.T) {
	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api := matrixapi.NewServer(nil).WithTraceStorage(memstore.New())
	if err := startLocalNotificationServer(ctx, home, api); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "data", "run-notifications.sock")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode: %v %v", info, err)
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	resp, err := client.Get("http://unix/v1/run-notifications")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("socket response: %d", resp.StatusCode)
	}
}
