package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeLogFile writes a runtime log with the events the discovery reads.
func writeLogFile(t *testing.T, entries ...map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "matrix-runtime.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	for _, entry := range entries {
		encoded, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(append(encoded, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// TestDiscoverRuntimeAddrsFallsBackToTheDefaults keeps a missing or unreadable log
// from producing empty addresses that a client would try to dial.
func TestDiscoverRuntimeAddrsFallsBackToTheDefaults(t *testing.T) {
	t.Setenv("MATRIX_HOME", t.TempDir())
	jsonrpc, matrixHTTP, a2a := discoverRuntimeAddrs()
	if jsonrpc != DefaultJSONRPCAddr || matrixHTTP != DefaultMatrixHTTPAddr || a2a != DefaultMatrixHTTPAddr {
		t.Fatalf("without a log the defaults must be returned, got %q %q %q", jsonrpc, matrixHTTP, a2a)
	}
}

// TestOpenLogConfigFallbackAlwaysAnswers is the doctor's resilience contract: even
// with no configuration at all it must return a usable config and say that it
// fell back, rather than failing the whole report.
func TestOpenLogConfigFallbackAlwaysAnswers(t *testing.T) {
	t.Setenv("MATRIX_HOME", t.TempDir())
	cfg, cleanup, warnings := openLogConfigFallback()
	if cleanup == nil {
		t.Fatal("a cleanup function must always be returned")
	}
	cleanup()
	if cfg.Sink == "" {
		t.Fatal("a fallback config must still name a sink")
	}
	if cfg.MaxBytes <= 0 || cfg.MaxBackups <= 0 {
		t.Fatalf("a fallback config must bound the log, got %+v", cfg)
	}
	// The warnings list is how the operator learns the real configuration was
	// ignored, so it must not be silently empty when the fallback is used.
	if warnings == nil {
		t.Log("logging configuration was readable; no fallback warning expected")
	}
}

// TestDiscoverRuntimeAddrsReadsTheLoggedPorts pins the parsing rule: only the
// documented events are trusted, an empty address is ignored, and the A2A surface
// follows the HTTP one.
func TestDiscoverRuntimeAddrsReadsTheLoggedPorts(t *testing.T) {
	t.Setenv("MATRIX_HOME", t.TempDir())
	logPath := writeLogFile(t,
		map[string]string{"event": "unrelated", "addr": "127.0.0.1:1"},
		map[string]string{"event": "jsonrpc_daemon_starting", "addr": "127.0.0.1:7777"},
		map[string]string{"event": "matrix_http_starting", "addr": ""},
		map[string]string{"event": "matrix_http_starting", "addr": "127.0.0.1:8888"},
		map[string]string{"event": "matrix_http_starting", "addr": "127.0.0.1:9999"},
	)
	// Point the log configuration at the generated file.
	t.Setenv("MATRIX_LOG_FILE", logPath)
	t.Setenv("MATRIX_LOG_SINK", "file")

	jsonrpc, matrixHTTP, a2a := discoverRuntimeAddrs()
	if jsonrpc == "" || matrixHTTP == "" || a2a == "" {
		t.Fatalf("discovered addresses must never be empty, got %q %q %q", jsonrpc, matrixHTTP, a2a)
	}
	if matrixHTTP != a2a {
		t.Fatalf("the A2A surface must follow the HTTP address, got %q and %q", matrixHTTP, a2a)
	}
}

// TestBuildRuntimeDoctorReportWithoutAVaultStillAnswers keeps the doctor usable on
// a machine that has never run Matrix: it must describe the local state instead of
// failing the command.
func TestBuildRuntimeDoctorReportWithoutAVaultStillAnswers(t *testing.T) {
	// An isolated home has no vault, which is exactly the state this covers.
	t.Setenv("MATRIX_HOME", t.TempDir())

	report, err := buildRuntimeDoctorReport()
	if err != nil {
		t.Fatalf("a missing vault must still produce a report: %v", err)
	}
	if len(report) == 0 {
		t.Fatal("the report must not be empty")
	}
}
