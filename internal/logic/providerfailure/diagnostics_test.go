package providerfailure

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

// codedRPCError stands in for any protocol SDK's coded error. It is deliberately
// a local type: this layer must read a coded error structurally, without
// importing the protocol that produced it.
type codedRPCError struct {
	code    int
	message string
	data    any
}

func (e *codedRPCError) Error() string { return fmt.Sprintf("RPC error %d: %s", e.code, e.message) }
func (e *codedRPCError) RPCErrorCode() int {
	return e.code
}
func (e *codedRPCError) RPCErrorMessage() string { return e.message }
func (e *codedRPCError) RPCErrorData() any       { return e.data }

// TestRPCErrorDiagnosticsSurviveToTheTrace is the regression guard for the
// hand-off that made an ACP failure unreadable: the peer's code, message and
// payload were only reachable by parsing a rendered sentence. A consumer must be
// able to read them as fields, from the trace event itself.
func TestRPCErrorDiagnosticsSurviveToTheTrace(t *testing.T) {
	store := runtrace.NewStore(memstore.New())
	run, _, err := store.Start(runtrace.Run{
		AgentID: "agent", Protocol: "acp", ChannelID: "http.test", ExecutionMode: runtrace.ExecutionModeSync,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}

	cause := &codedRPCError{
		code:    -32603,
		message: "Internal error",
		data:    map[string]any{"session": "ses_1", "phase": "new"},
	}
	failure := &Failure{
		Code:        PreflightFailed,
		Message:     "agent provider preflight failed",
		AgentID:     "agent",
		Protocol:    "acp",
		Phase:       "session/new",
		Diagnostics: Diagnostics(middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio"}, cause),
		Err:         cause,
	}
	AppendRunEvent(store, run.ID, failure)

	events, err := store.LoadEvents(run.ID, 0)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	var event *runtrace.Event
	for i := range events {
		if events[i].Kind == "provider.preflight.failed" {
			event = &events[i]
		}
	}
	if event == nil {
		t.Fatalf("expected a provider.preflight.failed event, got %+v", events)
	}
	metadata := event.Metadata
	if metadata["rpc_error_code"] != "-32603" {
		t.Fatalf("ACP code did not survive as a field: %+v", metadata)
	}
	if metadata["rpc_error_message"] != "Internal error" {
		t.Fatalf("ACP message did not survive as a field: %+v", metadata)
	}
	if payload, _ := metadata["rpc_error_data"].(string); !strings.Contains(payload, "ses_1") {
		t.Fatalf("ACP payload did not survive as a field: %+v", metadata)
	}
	if metadata["code"] != PreflightFailed || metadata["phase"] != "session/new" {
		t.Fatalf("existing failure fields changed: %+v", metadata)
	}
}

// TestUnclassifiedRPCFailureIsNamedAsSuch covers the fallback: an RPC failure
// that no classification recognises still reports the protocol's own reason
// instead of degrading to a generic provider error.
func TestUnclassifiedRPCFailureIsNamedAsSuch(t *testing.T) {
	diagnostics := Diagnostics(
		middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio"},
		&codedRPCError{code: -32000, message: "nope"},
	)
	if diagnostics["failure_reason"] != "provider_rpc_error" {
		t.Fatalf("expected an rpc failure reason, got %+v", diagnostics)
	}
	if diagnostics["rpc_error_code"] != "-32000" {
		t.Fatalf("expected the code, got %+v", diagnostics)
	}
}

// TestDiagnosticsNeverRenderAnEmptyPayloadAsMapText is the C3 half that lives in
// this layer: whatever a peer's payload was, the diagnostics a trace shows must
// not carry Matrix's rendering of emptiness as if it were evidence.
func TestDiagnosticsNeverRenderAnEmptyPayloadAsMapText(t *testing.T) {
	diagnostics := Diagnostics(
		middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio"},
		&codedRPCError{code: -32603, message: "Internal error", data: map[string]any{}},
	)
	for key, value := range diagnostics {
		if strings.Contains(value, "map[]") {
			t.Fatalf("diagnostic %q renders an empty payload: %q", key, value)
		}
	}
	if diagnostics["provider_error"] != "RPC error -32603: Internal error" {
		t.Fatalf("provider_error changed: %q", diagnostics["provider_error"])
	}
}

// TestDiagnosticsBoundAChattyPayload keeps a peer from turning a trace field into
// a transcript, and proves the cut is marked rather than silent.
func TestDiagnosticsBoundAChattyPayload(t *testing.T) {
	long := strings.Repeat("x", maxDiagnosticLength*2)
	diagnostics := Diagnostics(
		middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio"},
		&codedRPCError{code: -32603, message: long},
	)
	message := diagnostics["rpc_error_message"]
	if len(message) <= maxDiagnosticLength {
		t.Fatalf("expected the message to be cut, got %d bytes", len(message))
	}
	if !strings.HasSuffix(message, "…") {
		t.Fatalf("expected the cut to be marked: %q", message[maxDiagnosticLength-8:])
	}
}

// TestProcessFailureDiagnosticsStillWin proves the new RPC fields do not displace
// the process evidence an operator already relies on.
func TestProcessFailureDiagnosticsStillWin(t *testing.T) {
	diagnostics := Diagnostics(
		middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Transport: "stdio"},
		errors.New("signal: killed"),
	)
	if diagnostics["failure_reason"] != "provider_process_killed" {
		t.Fatalf("process classification changed: %+v", diagnostics)
	}
	if _, present := diagnostics["rpc_error_code"]; present {
		t.Fatalf("a non-RPC error produced RPC fields: %+v", diagnostics)
	}
}
