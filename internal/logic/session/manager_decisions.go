package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/jose/matrix-v2/internal/logic/workspace"
	"github.com/jose/matrix-v2/internal/middleware"
)

type routeDecision struct {
	Kind              string
	Source            string
	Explanation       string
	RequestedAgentID  string
	SelectedAgentID   string
	SelectedSessionID string
	SelectedMode      string
	FallbackUsed      bool
}

func (m *Manager) recordWorkspaceDecision(meta SessionMeta, channelID string, decision *routeDecision) {
	if decision == nil || meta.WorkspaceID == "" {
		return
	}
	metadata := map[string]interface{}{
		"kind":                strings.TrimSpace(decision.Kind),
		"source":              strings.TrimSpace(decision.Source),
		"explanation":         strings.TrimSpace(decision.Explanation),
		"requested_agent_id":  strings.TrimSpace(decision.RequestedAgentID),
		"selected_agent_id":   firstNonEmpty(strings.TrimSpace(decision.SelectedAgentID), meta.AgentID),
		"selected_session_id": firstNonEmpty(strings.TrimSpace(decision.SelectedSessionID), meta.ID),
		"selected_mode":       firstNonEmpty(strings.TrimSpace(decision.SelectedMode), normalizeMode(meta.Mode)),
		"fallback_used":       decision.FallbackUsed,
	}
	m.recordWorkspaceEvent(meta, "decision.recorded", channelID, strings.TrimSpace(decision.Explanation), strings.TrimSpace(decision.Kind), metadata)
}

func toDecisionTrace(raw map[string]interface{}) *middleware.WorkspaceDecisionTrace {
	if len(raw) == 0 {
		return nil
	}
	trace := &middleware.WorkspaceDecisionTrace{
		Kind:              stringify(raw["kind"]),
		Source:            stringify(raw["source"]),
		Explanation:       stringify(raw["explanation"]),
		RequestedAgentID:  stringify(raw["requested_agent_id"]),
		SelectedAgentID:   stringify(raw["selected_agent_id"]),
		SelectedSessionID: stringify(raw["selected_session_id"]),
		SelectedMode:      stringify(raw["selected_mode"]),
		CreatedAt:         stringify(raw["created_at"]),
	}
	if value, ok := raw["fallback_used"].(bool); ok {
		trace.FallbackUsed = value
	}
	if trace.Kind == "" && trace.Explanation == "" && trace.SelectedAgentID == "" && trace.SelectedSessionID == "" {
		return nil
	}
	return trace
}

func decisionTraceFromEvent(event workspace.Event) *middleware.WorkspaceDecisionTrace {
	if event.Type != "decision.recorded" {
		return nil
	}
	trace := toDecisionTrace(event.Metadata)
	if trace == nil {
		return nil
	}
	if trace.CreatedAt == "" {
		trace.CreatedAt = event.CreatedAt.Format(time.RFC3339)
	}
	return trace
}

func describeDecisionTrace(trace *middleware.WorkspaceDecisionTrace) string {
	if trace == nil {
		return "-"
	}
	line := strings.TrimSpace(trace.Explanation)
	if line == "" {
		line = fmt.Sprintf("%s via %s", valueOrDash(trace.Kind), valueOrDash(trace.Source))
	}
	if trace.SelectedAgentID != "" {
		line += " [agent: " + trace.SelectedAgentID + "]"
	}
	if trace.SelectedSessionID != "" {
		line += " [session: " + shortOrDash(trace.SelectedSessionID, 8) + "]"
	}
	if trace.FallbackUsed {
		line += " [fallback]"
	}
	return line
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
