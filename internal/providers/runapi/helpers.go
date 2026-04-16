package runapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/runtrace"
)

func newRunResponse(runID, status, output string) runResponse {
	return runResponse{
		RunID:      runID,
		Status:     status,
		Output:     output,
		TraceURL:   RunResourcePrefixV1 + runID + "/trace",
		EventsURL:  RunResourcePrefixV1 + runID + "/events",
		ActionsURL: RunResourcePrefixV1 + runID + "/actions",
	}
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("matrix http response encode failed", "error", err)
	}
}

func parseRunResource(path string) (string, string, bool) {
	rest := strings.TrimPrefix(path, RunResourcePrefixV1)
	if rest == path {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func normalizeRunExecutionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case runtrace.ExecutionModeAsync:
		return runtrace.ExecutionModeAsync
	case runtrace.ExecutionModeStream:
		return runtrace.ExecutionModeStream
	default:
		return runtrace.ExecutionModeSync
	}
}

func (s *Server) resolveProtocol(agentID string) string {
	if s.endpointResolver == nil || strings.TrimSpace(agentID) == "" {
		return "matrix"
	}
	endpoint, err := s.endpointResolver.GetAgentEndpoint(agentID)
	if err != nil || endpoint.Kind == "" {
		return "matrix"
	}
	return string(endpoint.Kind)
}

func inferToolName(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	for _, sep := range []string{" ", "\n", ":", "("} {
		if idx := strings.Index(content, sep); idx > 0 {
			return strings.TrimSpace(content[:idx])
		}
	}
	if len(content) > 64 {
		return content[:64]
	}
	return content
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parsePositiveInt(raw string) int {
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}
