package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// TestRuntimeDiscoveryPrefersWhatTheDaemonPublished is the acceptance evidence
// for client discovery: the address comes from the descriptor the daemon wrote,
// labelled as the published one, and the runtime log — which is where a client
// used to have to look — does not decide anything.
func TestRuntimeDiscoveryPrefersWhatTheDaemonPublished(t *testing.T) {
	home := t.TempDir()
	fs := osfs.NewFSProvider()
	if err := os.MkdirAll(filepath.Join(home, "data"), 0o700); err != nil {
		t.Fatal(err)
	}

	descriptor, err := runtimebroker.New("127.0.0.1:9190", filepath.Join(home, "logs", "runtime.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimebroker.Write(fs, runtimebroker.Path(home), descriptor); err != nil {
		t.Fatal(err)
	}

	// A log that claims a different port. Discovery must not read it: a log
	// records what a process bound once, not what is bound now.
	logPath := filepath.Join(home, "logs", "runtime.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte(`{"event":"jsonrpc_daemon_starting","addr":"127.0.0.1:9999"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	surfaces, notes := discoverRuntimeSurfaces(runtimeDiscoveryInput{
		Home:           home,
		ConfiguredRPC:  "127.0.0.1:9191",
		ConfiguredHTTP: "127.0.0.1:9192",
	}, fs)

	rpc := surfaceByID(t, surfaces, "jsonrpc")
	if rpc.Address != "127.0.0.1:9190" || rpc.Source != "runtime-broker" {
		t.Fatalf("jsonrpc surface = %s from %s, want the published 127.0.0.1:9190", rpc.Address, rpc.Source)
	}
	if rpc.Auth != "broker-token" || rpc.TokenFile != runtimebroker.Path(home) {
		t.Fatalf("the broker surface must name its credential and where it lives, got auth=%s token_file=%s", rpc.Auth, rpc.TokenFile)
	}
	for _, surface := range surfaces {
		if strings.Contains(surface.Address, "9999") {
			t.Fatalf("a logged address reached the discovery result: %+v", surface)
		}
		if descriptor.Token != "" && surface.Warning != "" && strings.Contains(surface.Warning, descriptor.Token) {
			t.Fatalf("the broker token leaked into a report: %q", surface.Warning)
		}
	}
	if len(notes) == 0 {
		t.Fatal("a published descriptor must be reported as the source of the answer")
	}

	httpSurface := surfaceByID(t, surfaces, "matrix-http")
	if httpSurface.Address != "127.0.0.1:9192" || httpSurface.Source != "config:matrix_http_addr" {
		t.Fatalf("http surface = %s from %s, want the configured 127.0.0.1:9192", httpSurface.Address, httpSurface.Source)
	}
}

// TestRuntimeDiscoverySaysWhenItIsGuessing pins the honest fallback: with no
// published descriptor the answer is configuration or the documented default,
// labelled as such and warned about, never presented as observation.
func TestRuntimeDiscoverySaysWhenItIsGuessing(t *testing.T) {
	surfaces, _ := discoverRuntimeSurfaces(runtimeDiscoveryInput{Home: t.TempDir()}, osfs.NewFSProvider())

	rpc := surfaceByID(t, surfaces, "jsonrpc")
	if rpc.Address != DefaultJSONRPCAddr || rpc.Source != "default" {
		t.Fatalf("jsonrpc surface = %s from %s, want the default with its source", rpc.Address, rpc.Source)
	}
	if !strings.Contains(rpc.Warning, "no published runtime descriptor") {
		t.Fatalf("the fallback must be labelled, got warning %q", rpc.Warning)
	}

	configured, _ := discoverRuntimeSurfaces(runtimeDiscoveryInput{
		Home:          t.TempDir(),
		ConfiguredRPC: "127.0.0.1:9290",
	}, osfs.NewFSProvider())
	if got := surfaceByID(t, configured, "jsonrpc"); got.Address != "127.0.0.1:9290" || got.Source != "config:jsonrpc_addr" {
		t.Fatalf("configured jsonrpc surface = %s from %s", got.Address, got.Source)
	}
}

// TestRuntimeDiscoveryReportsExposureAndCredential reuses the daemon's own bind
// rule: an external surface without its key is reported as unsafe rather than
// handed to a client as ready to use.
func TestRuntimeDiscoveryReportsExposureAndCredential(t *testing.T) {
	for _, test := range []struct {
		why          string
		addr         string
		apiKey       string
		wantExposure string
		wantAuth     string
		wantWarning  string
	}{
		{why: "loopback without a key", addr: "127.0.0.1:9090", wantExposure: "loopback", wantAuth: "none"},
		{why: "localhost without a key", addr: "localhost:9090", wantExposure: "loopback", wantAuth: "none"},
		{why: "external without a key", addr: "0.0.0.0:9090", wantExposure: "external", wantAuth: "none", wantWarning: "not loopback"},
		{why: "external with a key", addr: "0.0.0.0:9090", apiKey: "secret", wantExposure: "external", wantAuth: "api-key:X-Matrix-Key"},
	} {
		t.Run(test.why, func(t *testing.T) {
			surfaces, _ := discoverRuntimeSurfaces(runtimeDiscoveryInput{
				Home:           t.TempDir(),
				ConfiguredRPC:  test.addr,
				ConfiguredHTTP: test.addr,
				RPCKey:         test.apiKey,
				HTTPKey:        test.apiKey,
			}, osfs.NewFSProvider())
			for _, surface := range surfaces {
				if surface.Exposure != test.wantExposure {
					t.Fatalf("%s exposure = %s, want %s", surface.Kind, surface.Exposure, test.wantExposure)
				}
				if surface.Auth != test.wantAuth {
					t.Fatalf("%s auth = %s, want %s", surface.Kind, surface.Auth, test.wantAuth)
				}
				if test.wantWarning == "" && strings.Contains(surface.Warning, "not loopback") {
					t.Fatalf("%s was warned about although it is reachable safely: %q", surface.Kind, surface.Warning)
				}
				if test.wantWarning != "" && !strings.Contains(surface.Warning, test.wantWarning) {
					t.Fatalf("%s warning = %q, want it to mention %q", surface.Kind, surface.Warning, test.wantWarning)
				}
			}
		})
	}
}

// TestRuntimeDiscoveryKeepsTheTwoAPIKeysApart pins the defect class that made
// every local command answer 401: the Matrix HTTP surface and the JSON-RPC
// broker are authenticated by two keys that are configured and generated
// separately, so one surface's key must never stand in for the other's.
func TestRuntimeDiscoveryKeepsTheTwoAPIKeysApart(t *testing.T) {
	surfaces, _ := discoverRuntimeSurfaces(runtimeDiscoveryInput{
		Home:           t.TempDir(),
		ConfiguredRPC:  "0.0.0.0:9090",
		ConfiguredHTTP: "0.0.0.0:9091",
		RPCKey:         "broker-key",
		HTTPKey:        "",
	}, osfs.NewFSProvider())

	rpc := surfaceByID(t, surfaces, "jsonrpc")
	if rpc.Auth != "api-key:X-Matrix-Key" {
		t.Fatalf("jsonrpc auth = %s, want the broker key it was configured with", rpc.Auth)
	}
	if strings.Contains(rpc.Warning, "not loopback") {
		t.Fatalf("jsonrpc was warned about although its own key is set: %q", rpc.Warning)
	}

	httpSurface := surfaceByID(t, surfaces, "matrix-http")
	if httpSurface.Auth != "none" {
		t.Fatalf("matrix-http auth = %s, want none: the broker key is not its credential", httpSurface.Auth)
	}
	if !strings.Contains(httpSurface.Warning, "matrix_api_key") {
		t.Fatalf("matrix-http must be warned about with its own key name, got %q", httpSurface.Warning)
	}
}

func surfaceByID(t *testing.T, surfaces []runtimeSurface, id string) runtimeSurface {
	t.Helper()
	for _, surface := range surfaces {
		if surface.Kind == id {
			return surface
		}
	}
	t.Fatalf("no %s surface in %+v", id, surfaces)
	return runtimeSurface{}
}
