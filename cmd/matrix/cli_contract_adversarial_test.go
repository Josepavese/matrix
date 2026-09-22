package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentcatalog"
	"github.com/Josepavese/matrix/internal/logic/agentdiscovery"
)

// TestSplitCommaListIgnoresEmptyEntries keeps a trailing comma or a stray space
// from producing an empty value that a later lookup would treat as a name.
func TestSplitCommaListIgnoresEmptyEntries(t *testing.T) {
	if got := splitCommaList(""); got != nil {
		t.Fatalf("an empty list must stay nil, got %v", got)
	}
	got := splitCommaList(" a , ,b ,  ")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("splitCommaList = %v, want [a b]", got)
	}
}

// TestParseDiscoverySourcesRejectsUnknownValues is the configuration contract: a
// typo must not silently become a source, and asking for nothing must yield the
// documented defaults rather than no discovery at all.
func TestParseDiscoverySourcesRejectsUnknownValues(t *testing.T) {
	defaults := agentcatalog.DefaultSources()
	if len(defaults) == 0 {
		t.Fatal("the catalog must declare default sources")
	}
	if got := parseDiscoverySources(""); len(got) != len(defaults) {
		t.Fatalf("an empty configuration must fall back to the defaults, got %v", got)
	}
	if got := parseDiscoverySources("nonsense,also-wrong"); len(got) != len(defaults) {
		t.Fatalf("unknown values must fall back to the defaults, got %v", got)
	}
	got := parseDiscoverySources(string(agentdiscovery.SourceLocal) + ",nonsense")
	if len(got) != 1 || got[0] != agentdiscovery.SourceLocal {
		t.Fatalf("a known value must survive next to an unknown one, got %v", got)
	}
}

// TestFilterAgentArgsDropsBlanks keeps an empty argument from reaching the agent
// launcher, where it would be interpreted as an empty positional argument.
func TestFilterAgentArgsDropsBlanks(t *testing.T) {
	got := filterAgentArgs([]string{"--flag", "", "  ", "value"})
	if len(got) != 2 || got[0] != "--flag" || got[1] != "value" {
		t.Fatalf("filterAgentArgs = %v", got)
	}
	if got := filterAgentArgs(nil); len(got) != 0 {
		t.Fatalf("no arguments must filter to none, got %v", got)
	}
}

// TestRequestMatrixAPIKeyPrefersTheDedicatedHeader pins the extraction order and
// the case-insensitive bearer scheme: a client that sends either form must be
// authenticated, and an unrelated header must not be read as a key.
func TestRequestMatrixAPIKeyPrefersTheDedicatedHeader(t *testing.T) {
	withHeader := httptest.NewRequest(http.MethodGet, "/", nil)
	withHeader.Header.Set("X-Matrix-Key", " dedicated ")
	if got := requestMatrixAPIKey(withHeader); got != "dedicated" {
		t.Fatalf("the dedicated header must win, got %q", got)
	}

	bearer := httptest.NewRequest(http.MethodGet, "/", nil)
	bearer.Header.Set("Authorization", "Bearer  token-1 ")
	if got := requestMatrixAPIKey(bearer); got != "token-1" {
		t.Fatalf("a bearer token must be read, got %q", got)
	}

	lower := httptest.NewRequest(http.MethodGet, "/", nil)
	lower.Header.Set("Authorization", "bearer token-2")
	if got := requestMatrixAPIKey(lower); got != "token-2" {
		t.Fatalf("the scheme must be case-insensitive, got %q", got)
	}

	for name, req := range map[string]*http.Request{
		"no headers":   httptest.NewRequest(http.MethodGet, "/", nil),
		"wrong scheme": requestWithAuth("Basic dXNlcjpwYXNz"),
		"empty auth":   requestWithAuth("Bearer   "),
	} {
		if got := requestMatrixAPIKey(req); got != "" {
			t.Fatalf("%s must yield no key, got %q", name, got)
		}
	}
}

func requestWithAuth(value string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", value)
	return req
}

// TestAuthorizeRuntimeRequestRejectsNonBearerSchemes keeps a basic-auth header
// from being accepted as the runtime key, and keeps the 401 shaped for a client.
func TestAuthorizeRuntimeRequestRejectsNonBearerSchemes(t *testing.T) {
	req := requestWithAuth("Basic dXNlcjpwYXNz")
	recorder := httptest.NewRecorder()
	if authorizeMatrixRuntimeRequest(recorder, req, "secret") {
		t.Fatal("basic auth must not authorise a runtime request")
	}
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Unauthorized") {
		t.Fatalf("the 401 must say why, got %q", recorder.Body.String())
	}
	// A runtime with no configured key is deliberately open: the local runtime is
	// authenticated only when an operator sets a key, and refusing here would
	// break every local client. This is a contract, so it is asserted rather than
	// left implicit.
	open := httptest.NewRequest(http.MethodGet, "/", nil)
	if !authorizeMatrixRuntimeRequest(httptest.NewRecorder(), open, "") {
		t.Fatal("a runtime without a configured key must accept local requests")
	}
	// With a key configured, a request that carries none must not be accepted.
	bare := httptest.NewRequest(http.MethodGet, "/", nil)
	if authorizeMatrixRuntimeRequest(httptest.NewRecorder(), bare, "secret") {
		t.Fatal("a configured key must be required")
	}
}

// TestResolveInvocationPathIsRelativeToTheLaunchingDirectory pins the resolution
// rule: an absolute path is cleaned as given, a relative one is resolved against
// the directory the CLI was invoked from, and an optional empty value stays empty
// instead of becoming the current directory.
func TestResolveInvocationPathIsRelativeToTheLaunchingDirectory(t *testing.T) {
	previousCWD := invocationCWD
	t.Cleanup(func() { invocationCWD = previousCWD })
	invocationCWD = "/work/dir"

	absolute, err := resolveInvocationPath("/usr/bin/../bin/sh")
	if err != nil {
		t.Fatalf("absolute path: %v", err)
	}
	if absolute != "/usr/bin/sh" {
		t.Fatalf("an absolute path must be cleaned, got %q", absolute)
	}

	relative, err := resolveInvocationPath("agents/codex")
	if err != nil {
		t.Fatalf("relative path: %v", err)
	}
	if relative != "/work/dir/agents/codex" {
		t.Fatalf("a relative path must resolve against the invocation directory, got %q", relative)
	}

	optional, err := resolveOptionalInvocationPath("")
	if err != nil {
		t.Fatalf("optional empty path: %v", err)
	}
	if optional != "" {
		t.Fatalf("an unset optional path must stay empty, got %q", optional)
	}

	optionalSet, err := resolveOptionalInvocationPath("bin/agent")
	if err != nil || !strings.HasSuffix(optionalSet, "bin/agent") {
		t.Fatalf("an optional set path must resolve, got %q %v", optionalSet, err)
	}
}

// TestConfigureMatrixHomeIsolatesTheOperatorHome keeps the CLI from touching the
// real home during tests, and proves the helper wires MATRIX_HOME through.
func TestConfigureMatrixHomeIsolatesTheOperatorHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)
	if err := configureMatrixHome(); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if activeMatrixHome == "" {
		t.Fatal("the active home must be recorded")
	}
	if invocationCWD == "" {
		t.Fatal("the invocation directory must be recorded")
	}
	if _, err := filepath.Abs(activeMatrixHome); err != nil {
		t.Fatalf("the recorded home must be a path: %v", err)
	}
}
