// Package runtimeendpoints answers one question for a client: how do I reach
// this runtime, and what does reaching it cost. The answer is built from what the
// daemon published, then from the installation's configuration, then from the
// documented default, and every address is labelled with which of the three it
// came from. The runtime log is never read.
package runtimeendpoints

import (
	"fmt"
	"net"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
	"github.com/Josepavese/matrix/internal/logic/runtimecheck"
	"github.com/Josepavese/matrix/internal/middleware"
)

// Discovery for a client, not for a human reading a log.
//
// A supervisor that has to reach Matrix must learn three things: which address
// speaks which transport, what credential that address expects, and whether
// reaching it exposes anything. It must learn them from what the daemon
// publishes, because the two failure modes this replaces are guessing the port
// (JSON-RPC and the HTTP API are different ports on the same host) and reading
// the runtime log to find out what the daemon bound.
//
// The published descriptor is the existing contract: runtimebroker writes
// <home>/data/runtime-broker.json with the address it bound and the token it
// accepts. This file reads that contract and adds nothing to it.

const (
	// DefaultJSONRPCAddr and DefaultMatrixHTTPAddr are the addresses a local
	// runtime binds when the installation configures none. They are published
	// here with the discovery that uses them, so a client and the daemon cannot
	// disagree about where the default runtime is.
	DefaultJSONRPCAddr    = "127.0.0.1:9090"
	DefaultMatrixHTTPAddr = "127.0.0.1:9091"

	// runtimeRPCTransport is how the broker address is spoken: JSON-RPC carried
	// over HTTP on the daemon's own port.
	runtimeRPCTransport = "http+jsonrpc"
	// runtimeHTTPTransport is the Matrix inbound HTTP API.
	runtimeHTTPTransport = "http"
)

// Surface is one address a client can reach, with what it speaks, where
// the address came from, and what reaching it costs.
//
// Kind names the CLASS of surface (the JSON-RPC broker, the Matrix HTTP API),
// not a particular surface, and each kind carries the configuration key that
// authenticates it. Nothing here resolves a credential by comparing a surface
// against a literal: the credential is decided where the surface is built.
type Surface struct {
	Kind      string `json:"kind"`
	Transport string `json:"transport"`
	Address   string `json:"address"`
	Source    string `json:"source"`
	Auth      string `json:"auth"`
	Exposure  string `json:"exposure"`
	TokenFile string `json:"token_file,omitempty"`
	Warning   string `json:"warning,omitempty"`

	// authKey and authKeyName are the configured credential and the key it is
	// read from, kept out of the report. They exist so the bind rule is applied
	// with the credential the surface actually uses instead of a key borrowed
	// from the other surface.
	authKey     string
	authKeyName string
}

// Input is everything the resolution needs, so the rules can be
// exercised without a daemon, a vault or a network.
//
// The two surfaces authenticate with two different keys, configured and
// generated separately: the Matrix HTTP API (and the local socket it serves)
// uses matrix_api_key, the JSON-RPC broker uses daemon_api_key. They are kept
// apart here so a client is never told to present the other surface's key.
type Input struct {
	Home           string
	ConfiguredRPC  string
	ConfiguredHTTP string
	RPCKey         string
	HTTPKey        string
}

// Discover resolves the surfaces in the order a client must
// trust them: what the daemon published, then what the installation configured,
// then the documented default — each labelled, so a client never has to assume
// which one it received.
func Discover(input Input, fs middleware.FS) ([]Surface, []string) {
	notes := []string{}
	rpc := Surface{
		Kind: "jsonrpc", Transport: runtimeRPCTransport,
		authKey: input.RPCKey, authKeyName: "daemon_api_key",
	}

	if descriptor, err := runtimebroker.Read(fs, runtimebroker.Path(input.Home)); err == nil {
		rpc.Address = descriptor.JSONRPCAddr
		rpc.Source = "runtime-broker"
		rpc.Auth = "broker-token"
		rpc.TokenFile = runtimebroker.Path(input.Home)
		notes = append(notes, fmt.Sprintf("published descriptor: pid %d, started %s", descriptor.PID, descriptor.StartedAt.UTC().Format("2006-01-02T15:04:05Z")))
	} else {
		rpc.Warning = "no published runtime descriptor: " + err.Error() + "; the address below is configuration, not observation"
		if configured := strings.TrimSpace(input.ConfiguredRPC); configured != "" {
			rpc.Address, rpc.Source = configured, "config:jsonrpc_addr"
		} else {
			rpc.Address, rpc.Source = DefaultJSONRPCAddr, "default"
		}
		rpc.Auth = apiKeyAuth(input.RPCKey)
	}

	httpSurface := Surface{
		Kind: "matrix-http", Transport: runtimeHTTPTransport,
		Auth: apiKeyAuth(input.HTTPKey), authKey: input.HTTPKey, authKeyName: "matrix_api_key",
	}
	if configured := strings.TrimSpace(input.ConfiguredHTTP); configured != "" {
		httpSurface.Address, httpSurface.Source = configured, "config:matrix_http_addr"
	} else {
		httpSurface.Address, httpSurface.Source = DefaultMatrixHTTPAddr, "default"
	}

	surfaces := []Surface{rpc, httpSurface}
	for index := range surfaces {
		surfaces[index].Exposure = exposureOf(surfaces[index].Address)
		surfaces[index].Warning = strings.Join(nonEmpty(surfaces[index].Warning, exposureWarning(surfaces[index])), "; ")
	}
	return surfaces, notes
}

// exposureWarning applies the same rule the daemon applies before it binds:
// a non-loopback address without an API key is refused, so a client must not be
// told the surface is usable as configured.
func exposureWarning(surface Surface) string {
	if surface.Exposure != "external" || surface.Auth != "none" {
		return ""
	}
	keyName := surface.authKeyName
	if err := runtimecheck.RequireAPIKeyForExternalBind(surface.Address, surface.authKey, keyName, keyName); err != nil {
		return err.Error()
	}
	return ""
}

func apiKeyAuth(apiKey string) string {
	if strings.TrimSpace(apiKey) == "" {
		return "none"
	}
	return "api-key:X-Matrix-Key"
}

// exposureOf classifies an address by whether it leaves the host. A malformed
// address is reported as external: the classification is used to warn, and an
// unreadable address is not evidence of safety.
func exposureOf(address string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return "external"
	}
	if strings.EqualFold(host, "localhost") {
		return "loopback"
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil && ip.IsLoopback() {
		return "loopback"
	}
	return "external"
}

func nonEmpty(values ...string) []string {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			kept = append(kept, value)
		}
	}
	return kept
}
