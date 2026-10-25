package agents

import (
	"strings"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

func classifyProviderFailure(agentID string, endpoint middleware.ProtocolEndpoint, phase string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := providerfailure.As(err); ok {
		return err
	}

	errText := err.Error()
	code, message := providerfailure.DefaultClassification(phase, err)
	model := ""
	lower := strings.ToLower(errText)
	switch {
	case strings.Contains(lower, "does not advertise additionaldirectories"):
		code = providerfailure.AdditionalDirectoriesUnsupported
		message = "agent does not support the requested additional directories"
	case strings.Contains(lower, "directory not authorized") || strings.Contains(lower, "workspace not trusted") || strings.Contains(lower, "untrusted workspace"):
		code = providerfailure.WorkspaceRejected
		message = "provider refused the requested workspace; complete the provider trust flow"
	case zedacp.IsAuthenticationRequired(err):
		// ACP v2's structured signal, checked before the text heuristics.
		code = providerfailure.AuthMismatch
		message = "the agent requires authentication before this request"
	case isModelUnavailableError(lower):
		code = providerfailure.ModelUnavailable
		message = "configured provider model is unavailable through the selected adapter"
		model = extractBacktickModel(errText)
	case isToolPermissionDenial(lower):
		code = providerfailure.ProviderToolPermissionDenied
		message = "provider aborted the turn after a tool permission was denied"
	case isProviderAuthError(lower):
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
		Diagnostics:    providerfailure.Diagnostics(endpoint, err),
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
			strings.Contains(lower, "unavailable") ||
			strings.Contains(lower, "unknown model option") ||
			strings.Contains(lower, "do not have access") ||
			strings.Contains(lower, "not have access"))
}

// toolPermissionDenialSignals are the structured or protocol-shaped phrases a
// provider uses when it aborts a turn because a tool permission was denied. They
// are matched lowercased against the provider's error text; the plain "permission
// denied" phrase is also accepted because providers emit it without any tool
// noun, and an authentication refusal is excluded separately below. The final
// entries are Matrix's own denial wording for client-side fs/terminal methods,
// which the peer may echo into the abort it reports.
var toolPermissionDenialSignals = []string{
	"tool_permission_denied",
	"tool permission denied",
	"tool permission",
	"permission_denied",
	"permission denied",
	"user rejected",
	"rejected permission",
	"permission rejected",
	"denied permission",
	"user denied",
	"denied by user",
	"tool call rejected",
	"tool call was rejected",
	"rejected tool call",
	"agent.trust_mode is false",
}

// authenticationSignals mark a refusal as an authentication failure rather than a
// tool-permission denial. A provider that says both — for example "permission
// denied: invalid api key" — is an auth failure: the caller fixes credentials,
// not the tool policy. This is the conservative side of the classification,
// since a false tool-permission diagnosis would send a consumer looking for a
// denied tool that never existed.
var authenticationSignals = []string{
	"auth",
	"unauthorized",
	"forbidden",
	"api key",
	"apikey",
	"credential",
	"401",
	"403",
	"token",
	"login",
	"sign in",
}

// isToolPermissionDenial reports whether a provider turn error is evidenced as a
// denial of a tool permission. It never makes an error into a denial: only the
// provider's own words count, and authentication evidence always wins.
func isToolPermissionDenial(lower string) bool {
	if matchesAny(lower, authenticationSignals) {
		return false
	}
	return matchesAny(lower, toolPermissionDenialSignals)
}

func matchesAny(lower string, signals []string) bool {
	for _, signal := range signals {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
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
