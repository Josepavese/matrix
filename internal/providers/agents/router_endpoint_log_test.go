package agents

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// ----------------------------------------------------------------------------
// What the router writes about where a provider lives
//
// "resolved agent endpoint" is written on every client creation, whether or not
// anyone asked for it, so the address it carries goes through the redaction
// policy: withheld unless the operator opted in. These tests pin both halves of
// that, because a policy with only the redacted half would be indistinguishable
// from a log line that lost the value by accident.
// ----------------------------------------------------------------------------

// redactionEndpointResolver serves one endpoint whose address is a provider
// host, and whose kind no factory serves: createClient logs the resolution and
// then stops on the missing factory, so the assertion is about the log line
// without spawning anything or dialling anywhere.
type redactionEndpointResolver struct{ address string }

func (r redactionEndpointResolver) GetAgentEndpoint(string) (middleware.ProtocolEndpoint, error) {
	return middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKind("redaction_probe"),
		Transport: "stdio",
		Address:   r.address,
	}, nil
}

// captureRouterLog redirects the process logger for one test and returns the
// buffer the router writes into. The package's tests are sequential, so the
// default logger is not shared with a running neighbour.
func captureRouterLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buffer
}

// resolveForLogging runs the router's resolution path far enough to write the
// endpoint line, and asserts it got there.
func resolveForLogging(t *testing.T, address string) string {
	t.Helper()
	logs := captureRouterLog(t)
	router := NewRouter(redactionEndpointResolver{address: address})
	if _, _, err := router.createClient(context.Background(), "neutral-agent", t.TempDir()); err == nil {
		t.Fatal("the probe kind has no factory: createClient must stop after logging the endpoint")
	}
	line := strings.TrimSpace(logs.String())
	if !strings.Contains(line, "endpoint_resolved") {
		t.Fatalf("the endpoint resolution was not logged at all: %q", line)
	}
	return line
}

// TestResolvedEndpointIsLoggedRedactedByDefault is the default: the host is
// withheld from a record nobody requested, while the fields that describe the
// connection without naming the provider survive, so the log stays useful.
func TestResolvedEndpointIsLoggedRedactedByDefault(t *testing.T) {
	const host = "api.deepseek.com"
	line := resolveForLogging(t, host)

	if strings.Contains(line, host) {
		t.Fatalf("the provider host reached a log written without being asked for: %q", line)
	}
	if !strings.Contains(line, "address="+logRedactedPlaceholder) {
		t.Fatalf("the address must be present and withheld, got %q", line)
	}
	for _, field := range []string{"protocol_kind=redaction_probe", "transport=stdio", "command="} {
		if !strings.Contains(line, field) {
			t.Fatalf("withholding the address must not drop %q: %q", field, line)
		}
	}
}

// TestResolvedEndpointRevealIsTheOperatorsExplicitOptIn is the other half: the
// documented opt-in shows the address, so an operator debugging a provider
// connection can still see where Matrix is pointing.
func TestResolvedEndpointRevealIsTheOperatorsExplicitOptIn(t *testing.T) {
	const host = "api.deepseek.com"
	t.Setenv("MATRIX_LOG_REVEAL_ENDPOINTS", "1")
	line := resolveForLogging(t, host)

	if !strings.Contains(line, "address="+host) {
		t.Fatalf("the explicit opt-in must reveal the endpoint, got %q", line)
	}
	if strings.Contains(line, logRedactedPlaceholder) {
		t.Fatalf("with the opt-in on, nothing is withheld: %q", line)
	}
}

// logRedactedPlaceholder mirrors the policy package's replacement. It is a
// literal on purpose: this test asserts what an operator reads in a log, and
// importing the constant would let a change to it pass unnoticed.
const logRedactedPlaceholder = "***"
