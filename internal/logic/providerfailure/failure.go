package providerfailure

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/providerdiag"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/middleware"
)

const (
	ModelUnavailable = "provider_model_unavailable"
	AuthMismatch     = "provider_auth_mismatch"
	PreflightFailed  = "agent_preflight_failed"
)

type Failure struct {
	Code           string            `json:"code,omitempty"`
	Message        string            `json:"message,omitempty"`
	AgentID        string            `json:"agent_id,omitempty"`
	Protocol       string            `json:"protocol,omitempty"`
	Phase          string            `json:"phase,omitempty"`
	RequestedModel string            `json:"requested_model,omitempty"`
	Diagnostics    map[string]string `json:"diagnostics,omitempty"`
	Err            error             `json:"-"`
}

func (e *Failure) Error() string {
	base := fmt.Sprintf("[%s] %s", e.Code, e.Message)
	for _, part := range []struct{ key, value string }{
		{"agent", e.AgentID}, {"protocol", e.Protocol}, {"phase", e.Phase}, {"requested_model", e.RequestedModel},
	} {
		if part.value != "" {
			base += fmt.Sprintf(" %s=%s", part.key, part.value)
		}
	}
	if e.Err != nil {
		base += ": " + e.Err.Error()
	}
	return base
}

func (e *Failure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func As(err error) (*Failure, bool) {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure, true
	}
	return nil, false
}

func HTTPStatus(err error) (int, bool) {
	failure, ok := As(err)
	if !ok {
		return http.StatusInternalServerError, false
	}
	if failure.Code == ModelUnavailable || failure.Code == AuthMismatch {
		return http.StatusFailedDependency, true
	}
	return http.StatusBadGateway, true
}

func Details(failure *Failure) map[string]string {
	if failure == nil {
		return nil
	}
	details := map[string]string{}
	for key, value := range map[string]string{
		"agent_id": failure.AgentID, "protocol": failure.Protocol, "phase": failure.Phase, "requested_model": failure.RequestedModel,
	} {
		if value != "" {
			details[key] = value
		}
	}
	for key, value := range failure.Diagnostics {
		if value != "" {
			details[key] = value
		}
	}
	return details
}

func AppendProcessDiagnostics(diagnostics map[string]string, err error) {
	var failure *providerdiag.ProcessFailure
	if !errors.As(err, &failure) {
		return
	}
	diagnostics["failure_reason"] = "provider_process_exit"
	if failure.ExitCode >= 0 {
		diagnostics["provider_exit_code"] = strconv.Itoa(failure.ExitCode)
	}
	if failure.Stderr != "" {
		diagnostics["provider_stderr"] = failure.Stderr
	}
}

// NewPreflight builds a typed generic provider preflight failure.
func NewPreflight(agentID string, endpoint middleware.ProtocolEndpoint, phase string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := As(err); ok {
		return err
	}
	return &Failure{
		Code: PreflightFailed, Message: "agent provider preflight failed", AgentID: agentID,
		Protocol: string(endpoint.Kind), Phase: phase, Diagnostics: Diagnostics(endpoint, err), Err: err,
	}
}

// Diagnostics returns bounded launch and transport evidence for a provider error.
func Diagnostics(endpoint middleware.ProtocolEndpoint, err error) map[string]string {
	diagnostics := map[string]string{"transport": endpoint.Transport}
	if endpoint.Command != "" {
		diagnostics["command"] = endpoint.Command
		diagnostics["adapter"] = filepath.Base(endpoint.Command)
	}
	if endpoint.Address != "" {
		diagnostics["address"] = endpoint.Address
	}
	if endpoint.ProtocolVersion != "" {
		diagnostics["protocol_version"] = endpoint.ProtocolVersion
	}
	if err != nil {
		diagnostics["provider_error"] = err.Error()
		diagnostics["failure_reason"] = failureReason(err.Error())
		AppendProcessDiagnostics(diagnostics, err)
	}
	return diagnostics
}

func failureReason(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "client context cancelled") || strings.Contains(lower, "client context canceled"):
		return "provider_client_context_cancelled"
	case strings.Contains(lower, "context cancelled") || strings.Contains(lower, "context canceled"):
		return "request_context_cancelled"
	case strings.Contains(lower, "signal: killed"):
		return "provider_process_killed"
	case strings.Contains(lower, "exit status"):
		return "provider_process_exit"
	case strings.Contains(lower, "eof"):
		return "provider_transport_eof"
	case strings.Contains(lower, "broken pipe") || strings.Contains(lower, "file already closed"):
		return "provider_transport_closed"
	default:
		return "provider_error"
	}
}

func AppendRunEvent(store *runtrace.Store, runID string, err error) {
	failure, ok := As(err)
	if !ok || store == nil {
		return
	}
	metadata := map[string]interface{}{"code": failure.Code}
	for key, value := range Details(failure) {
		metadata[key] = value
	}
	_, _ = store.AppendEvent(runtrace.Event{
		RunID: runID, Kind: "provider.preflight.failed", Actor: "matrix", Status: runtrace.StatusFailed,
		Timestamp: time.Now().UTC(), Protocol: failure.Protocol, ProtocolMethod: failure.Phase,
		Message: failure.Message, Metadata: metadata,
	})
}
