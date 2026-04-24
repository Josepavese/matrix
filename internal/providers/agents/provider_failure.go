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
		Diagnostics:    providerFailureDiagnostics(endpoint),
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

func providerFailureDiagnostics(endpoint middleware.ProtocolEndpoint) map[string]string {
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
	return diagnostics
}
