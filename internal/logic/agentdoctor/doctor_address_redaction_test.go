package agentdoctor

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/logredact"
	"github.com/Josepavese/matrix/internal/middleware"
)

// TestDoctorReportsTheAddressLogsWithhold is the boundary of the redaction
// policy, stated where the two surfaces meet: a log is a record written without
// anyone asking for it, while `matrix doctor` is an answer to a question the
// operator asked. The same host must therefore be withheld in the first and
// shown in the second. A redaction applied to the report would make the
// diagnostic useless for the one thing it exists to check — where Matrix points
// — and this test fails if that happens.
func TestDoctorReportsTheAddressLogsWithhold(t *testing.T) {
	const host = "api.deepseek.com"
	t.Setenv("MATRIX_LOG_REVEAL_ENDPOINTS", "")

	if got := logredact.Endpoint(host); got != "***" {
		t.Fatalf("the default policy must withhold a host in a log, got %q", got)
	}

	endpoint := middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "http",
		Address:   host,
	}
	if got := EndpointAddress(endpoint); got != host {
		t.Fatalf("the doctor report must answer with the address it was asked for, got %q", got)
	}
}

// TestDoctorStillAnswersWithTheRevealOptIn is the same boundary from the other
// side, and it is the direction an over-eager implementation would break: with
// the operator's logging opt-in on, the report is unchanged — it never depended
// on the logging policy, so turning that policy around cannot turn the diagnosis
// into a placeholder.
func TestDoctorStillAnswersWithTheRevealOptIn(t *testing.T) {
	const host = "api.deepseek.com"
	t.Setenv("MATRIX_LOG_REVEAL_ENDPOINTS", "1")

	if got := logredact.Endpoint(host); got != host {
		t.Fatalf("the opt-in must reveal a host in a log, got %q", got)
	}

	endpoint := middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "http",
		Address:   host,
	}
	if got := EndpointAddress(endpoint); got != host {
		t.Fatalf("the doctor report must answer with the address it was asked for, got %q", got)
	}
}

// TestDoctorReportsTheLocalCommandForStdio is the same rule for the other shape
// the report handles: a stdio endpoint's target is the local program Matrix
// launches, and the operator asking for a diagnosis gets the path, not a
// placeholder.
func TestDoctorReportsTheLocalCommandForStdio(t *testing.T) {
	endpoint := middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "stdio",
		Command:   "/usr/bin/opencode",
		Address:   "api.deepseek.com",
	}
	if got := EndpointAddress(endpoint); got != "/usr/bin/opencode" {
		t.Fatalf("a stdio diagnosis must name the launched program, got %q", got)
	}
}
