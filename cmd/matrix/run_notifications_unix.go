//go:build linux || darwin

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Josepavese/matrix/internal/providers/matrixapi"
)

func startLocalNotificationServer(ctx context.Context, home string, api *matrixapi.Server) error {
	path := filepath.Join(home, "data", "run-notifications.sock")
	listener, owned, err := prepareNotificationListener(path)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	api.RegisterLocalNotificationRoutes(mux)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go serveNotificationSocket(server, listener, path, owned)
	return nil
}

func prepareNotificationListener(path string) (net.Listener, os.FileInfo, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, nil, fmt.Errorf("notification path exists and is not a socket: %s", path)
		}
		conn, err := net.DialTimeout("unix", path, 250*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil, nil, fmt.Errorf("notification socket already active: %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		return nil, nil, err
	}
	owned, _ := os.Lstat(path)
	return listener, owned, nil
}

func serveNotificationSocket(server *http.Server, listener net.Listener, path string, owned os.FileInfo) {
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		slog.Error("local notification socket stopped", "error", err)
	}
	if current, err := os.Lstat(path); err == nil && owned != nil && os.SameFile(owned, current) {
		_ = os.Remove(path)
	}
}
