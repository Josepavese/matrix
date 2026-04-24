package providerfailure

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
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
