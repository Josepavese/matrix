package agents

import (
	"path/filepath"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/middleware"
)

func classifyProviderFailure(agentID string, endpoint middleware.ProtocolEndpoint, phase string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := providerfailure.As(err); ok {
		return err
	}

	errText := err.Error()
	code := providerfailure.PreflightFailed
	message := "agent provider preflight failed"
	model := ""
	lower := strings.ToLower(errText)
	if isModelUnavailableError(lower) {
		code = providerfailure.ModelUnavailable
		message = "configured provider model is unavailable through the selected adapter"
		model = extractBacktickModel(errText)
	} else if isProviderAuthError(lower) {
		code = providerfailure.AuthMismatch
		message = "provider authentication failed or does not match the selected adapter"
	}

	return &providerfailure.Failure{
		Code:           code,
		Message:        message,
		AgentID:        agentID,
		Protocol:       string(endpoint.Kind),
		Phase:          phase,
		RequestedModel: model,
		Diagnostics:    providerFailureDiagnostics(endpoint, err),
		Err:            err,
	}
}

func annotateProviderFailureAgent(err error, agentID string) error {
	failure, ok := providerfailure.As(err)
	if !ok || failure.AgentID != "" {
		return err
	}
	next := *failure
	next.AgentID = agentID
	return &next
}

func isModelUnavailableError(lower string) bool {
	return strings.Contains(lower, "model") &&
		(strings.Contains(lower, "does not exist") ||
			strings.Contains(lower, "do not have access") ||
			strings.Contains(lower, "not have access"))
}

func isProviderAuthError(lower string) bool {
	return strings.Contains(lower, "auth") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "invalid api key")
}

func extractBacktickModel(text string) string {
	start := strings.Index(text, "`")
	if start < 0 {
		return ""
	}
	rest := text[start+1:]
	end := strings.Index(rest, "`")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func providerFailureDiagnostics(endpoint middleware.ProtocolEndpoint, err error) map[string]string {
	diagnostics := map[string]string{
		"transport": endpoint.Transport,
	}
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
		diagnostics["failure_reason"] = providerFailureReason(err.Error())
	}
	return diagnostics
}

func providerFailureReason(text string) string {
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
