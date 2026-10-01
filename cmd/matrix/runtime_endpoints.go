package main

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/cmdutil"
	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
	"github.com/Josepavese/matrix/internal/logic/runtimecheck"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/spf13/cobra"
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
	// runtimeRPCTransport is how the broker address is spoken: JSON-RPC carried
	// over HTTP on the daemon's own port.
	runtimeRPCTransport = "http+jsonrpc"
	// runtimeHTTPTransport is the Matrix inbound HTTP API.
	runtimeHTTPTransport = "http"
)

// runtimeSurface is one address a client can reach, with what it speaks, where
// the address came from, and what reaching it costs.
//
// Kind names the CLASS of surface (the JSON-RPC broker, the Matrix HTTP API),
// not a particular surface, and each kind carries the configuration key that
// authenticates it. Nothing here resolves a credential by comparing a surface
// against a literal: the credential is decided where the surface is built.
type runtimeSurface struct {
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

// runtimeDiscoveryInput is everything the resolution needs, so the rules can be
// exercised without a daemon, a vault or a network.
//
// The two surfaces authenticate with two different keys, configured and
// generated separately: the Matrix HTTP API (and the local socket it serves)
// uses matrix_api_key, the JSON-RPC broker uses daemon_api_key. They are kept
// apart here so a client is never told to present the other surface's key.
type runtimeDiscoveryInput struct {
	Home           string
	ConfiguredRPC  string
	ConfiguredHTTP string
	RPCKey         string
	HTTPKey        string
}

// discoverRuntimeSurfaces resolves the surfaces in the order a client must
// trust them: what the daemon published, then what the installation configured,
// then the documented default — each labelled, so a client never has to assume
// which one it received.
func discoverRuntimeSurfaces(input runtimeDiscoveryInput, fs middleware.FS) ([]runtimeSurface, []string) {
	notes := []string{}
	rpc := runtimeSurface{
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

	httpSurface := runtimeSurface{
		Kind: "matrix-http", Transport: runtimeHTTPTransport,
		Auth: apiKeyAuth(input.HTTPKey), authKey: input.HTTPKey, authKeyName: "matrix_api_key",
	}
	if configured := strings.TrimSpace(input.ConfiguredHTTP); configured != "" {
		httpSurface.Address, httpSurface.Source = configured, "config:matrix_http_addr"
	} else {
		httpSurface.Address, httpSurface.Source = DefaultMatrixHTTPAddr, "default"
	}

	surfaces := []runtimeSurface{rpc, httpSurface}
	for index := range surfaces {
		surfaces[index].Exposure = exposureOf(surfaces[index].Address)
		surfaces[index].Warning = strings.Join(nonEmpty(surfaces[index].Warning, exposureWarning(surfaces[index])), "; ")
	}
	return surfaces, notes
}

// exposureWarning applies the same rule the daemon applies before it binds:
// a non-loopback address without an API key is refused, so a client must not be
// told the surface is usable as configured.
func exposureWarning(surface runtimeSurface) string {
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

var runtimeEndpointsJSON bool

var runtimeEndpointsCmd = &cobra.Command{
	Use:   "endpoints",
	Short: "Show the runtime endpoints a client must use",
	Long: `Show how to reach this Matrix runtime.

The addresses come from what the daemon published (the runtime broker
descriptor) and from the installation's own configuration, each labelled with
its source. The runtime log is never parsed: a log records what happened once,
while the descriptor records what is bound now.

Nothing secret is printed. A surface that authenticates names the credential it
expects and, for the broker, the file the token lives in.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		if err := runRuntimeEndpoints(cmd); err != nil {
			exitf("Error: %v", err)
		}
	},
}

var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Inspect how clients reach this Matrix runtime",
}

func runRuntimeEndpoints(cmd *cobra.Command) error {
	home, err := resolveActiveHome()
	if err != nil {
		return err
	}
	manager, cleanup, err := cmdutil.OpenReadOnlyConfigManager(DefaultVaultPath)
	if err != nil {
		return fmt.Errorf("read runtime configuration: %w", err)
	}
	defer cleanup()

	surfaces, notes := discoverRuntimeSurfaces(runtimeDiscoveryInput{
		Home:           home,
		ConfiguredRPC:  manager.GetWithDefault("jsonrpc_addr", ""),
		ConfiguredHTTP: manager.GetWithDefault("matrix_http_addr", ""),
		RPCKey:         manager.GetWithDefault("daemon_api_key", ""),
		HTTPKey:        manager.GetWithDefault("matrix_api_key", ""),
	}, osfs.NewFSProvider())

	if runtimeEndpointsJSON {
		encoded, encodeErr := json.MarshalIndent(map[string]any{"surfaces": surfaces, "notes": notes}, "", "  ")
		if encodeErr != nil {
			return encodeErr
		}
		cmd.Println(string(encoded))
		return nil
	}
	for _, surface := range surfaces {
		cmd.Printf("%s transport=%s address=%s source=%s auth=%s exposure=%s\n",
			surface.Kind, surface.Transport, surface.Address, surface.Source, surface.Auth, surface.Exposure)
		if surface.TokenFile != "" {
			cmd.Printf("  token_file=%s\n", surface.TokenFile)
		}
		if surface.Warning != "" {
			cmd.Printf("  warning=%s\n", surface.Warning)
		}
	}
	for _, note := range notes {
		cmd.Printf("note=%s\n", note)
	}
	return nil
}

func init() {
	runtimeEndpointsCmd.Flags().BoolVar(&runtimeEndpointsJSON, "json", false, "print the machine-readable surface list")
	runtimeCmd.AddCommand(runtimeEndpointsCmd)
	rootCmd.AddCommand(runtimeCmd)
}
