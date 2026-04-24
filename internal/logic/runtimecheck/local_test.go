package runtimecheck

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func TestBuildLocalReportFallsBackWhenRuntimeEndpointReportIsInvalid(t *testing.T) {
	net := &localReportNet{
		canDial: true,
		report: map[string]any{
			"vault_exists":      true,
			"jsonrpc_daemon_up": true,
		},
	}
	report, err := BuildLocalReport(LocalInput{
		VaultPath:      "/tmp/matrix-vault.db",
		JSONRPCAddr:    "127.0.0.1:9090",
		MatrixHTTPAddr: "127.0.0.1:9091",
		A2AHTTPAddr:    "127.0.0.1:9091",
		Net:            net,
		FS:             &localReportFS{},
	})
	if err != nil {
		t.Fatalf("BuildLocalReport returned error: %v", err)
	}
	if got, _ := report["matrix_http_up"].(bool); !got {
		t.Fatalf("expected local probe bool fallback, got %T %[1]v", report["matrix_http_up"])
	}
	if !warningContains(report, "runtime endpoint report invalid") {
		t.Fatalf("expected invalid remote report warning, got %+v", report["warnings"])
	}
}

func TestBuildLocalReportUsesValidRuntimeEndpointReport(t *testing.T) {
	remote := map[string]any{
		"vault_exists":        true,
		"jsonrpc_daemon_up":   true,
		"matrix_http_up":      true,
		"a2a_http_up":         true,
		"jsonrpc_daemon_addr": "127.0.0.1:9090",
		"matrix_http_addr":    "127.0.0.1:9091",
		"a2a_http_addr":       "127.0.0.1:9091",
		"warnings":            []any{},
	}
	report, err := BuildLocalReport(LocalInput{
		VaultPath:      "/tmp/matrix-vault.db",
		JSONRPCAddr:    "127.0.0.1:9090",
		MatrixHTTPAddr: "127.0.0.1:9091",
		A2AHTTPAddr:    "127.0.0.1:9091",
		Net:            &localReportNet{canDial: true, report: remote},
		FS:             &localReportFS{},
	})
	if err != nil {
		t.Fatalf("BuildLocalReport returned error: %v", err)
	}
	if report["matrix_http_up"] != true {
		t.Fatalf("expected remote report, got %+v", report)
	}
	if warningContains(report, "fallback") {
		t.Fatalf("did not expect fallback warning, got %+v", report["warnings"])
	}
}

type localReportNet struct {
	canDial bool
	report  map[string]any
}

func (n *localReportNet) Listen(_, _ string) (middleware.ClosableListener, error) {
	return nil, errors.New("not implemented")
}

func (n *localReportNet) Download(context.Context, string, string) error { return nil }

func (n *localReportNet) FetchJSON(_ context.Context, _ string, target interface{}) error {
	report, ok := target.(*map[string]any)
	if !ok {
		return errors.New("unexpected target")
	}
	*report = n.report
	return nil
}

func (n *localReportNet) GetFreePort() (int, error) { return 0, nil }
func (n *localReportNet) Fetch(context.Context, string) ([]byte, error) {
	return nil, errors.New("not implemented")
}
func (n *localReportNet) PostJSON(context.Context, string, interface{}) ([]byte, int, error) {
	return nil, 0, errors.New("not implemented")
}
func (n *localReportNet) CanDial(string) bool { return n.canDial }

type localReportFS struct{}

func (f *localReportFS) Mount(string) error                   { return nil }
func (f *localReportFS) Unmount() error                       { return nil }
func (f *localReportFS) CreateDirectory(string) error         { return nil }
func (f *localReportFS) RemoveAll(string) error               { return nil }
func (f *localReportFS) Stat(string) (os.FileInfo, error)     { return nil, os.ErrNotExist }
func (f *localReportFS) MkdirAll(string, os.FileMode) error   { return nil }
func (f *localReportFS) UserHomeDir() (string, error)         { return "", nil }
func (f *localReportFS) TempDir() string                      { return "" }
func (f *localReportFS) Open(string) (middleware.File, error) { return noopFile{}, nil }
func (f *localReportFS) OpenFile(string, int, os.FileMode) (middleware.File, error) {
	return noopFile{}, nil
}
func (f *localReportFS) ReadFile(string) ([]byte, error) { return nil, nil }
func (f *localReportFS) Remove(string) error             { return nil }
func (f *localReportFS) Rename(string, string) error     { return nil }

type noopFile struct{}

func (noopFile) Read([]byte) (int, error)       { return 0, io.EOF }
func (noopFile) Write(p []byte) (int, error)    { return len(p), nil }
func (noopFile) Seek(int64, int) (int64, error) { return 0, nil }
func (noopFile) Close() error                   { return nil }

func warningContains(report map[string]any, text string) bool {
	switch warnings := report["warnings"].(type) {
	case []string:
		for _, warning := range warnings {
			if strings.Contains(warning, text) {
				return true
			}
		}
	case []any:
		for _, value := range warnings {
			if warning, ok := value.(string); ok && strings.Contains(warning, text) {
				return true
			}
		}
	}
	return false
}
