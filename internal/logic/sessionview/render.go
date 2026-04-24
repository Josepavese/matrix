package sessionview

import (
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

type StringLookup func(string) string
type SessionFormatter func(int, string, middleware.SessionEntry) string
type RemoteSessionFormatter func(int, middleware.RemoteSessionInfo) string

type RenderDeps struct {
	Lookup StringLookup
	Local  SessionFormatter
	Remote RemoteSessionFormatter
}

func RenderAction(result middleware.SessionActionResult, lang string, deps RenderDeps) string {
	if result.Message != "" && result.Action != "list" {
		return result.Message
	}
	switch result.Action {
	case "status":
		return renderStatus(result, deps.Lookup)
	case "list":
		return renderList(result, lang, deps)
	default:
		return result.Message
	}
}

func renderStatus(result middleware.SessionActionResult, lookup StringLookup) string {
	if result.Session == nil {
		return lookup("session_not_found_db")
	}
	details := statusDetails(result.Session)
	return fmt.Sprintf(lookup("session_status"), result.Session.LogicalSessionID, details, result.Session.AgentID, result.Session.CreatedAt)
}

func statusDetails(session *middleware.SessionEntry) string {
	var details strings.Builder
	appendDetail(&details, "Alias", quoted(session.Alias))
	appendDetail(&details, "Workspace", workspaceLabel(session))
	appendDetail(&details, "Mode", session.Mode)
	appendDetail(&details, "Handoff", handoffLabel(session))
	return details.String()
}

func appendDetail(details *strings.Builder, label, value string) {
	if value == "" {
		return
	}
	details.WriteString("\n")
	details.WriteString(label)
	details.WriteString(": ")
	details.WriteString(value)
}

func quoted(value string) string {
	if value == "" {
		return ""
	}
	return fmt.Sprintf("\"%s\"", value)
}

func workspaceLabel(session *middleware.SessionEntry) string {
	if session.WorkspaceID == "" {
		return ""
	}
	if session.WorkspacePath == "" {
		return session.WorkspaceID
	}
	return fmt.Sprintf("%s (%s)", session.WorkspaceID, session.WorkspacePath)
}

func handoffLabel(session *middleware.SessionEntry) string {
	if session.LastHandoff == nil || session.LastHandoff.FromAgentID == "" {
		return ""
	}
	return fmt.Sprintf("%s -> %s", session.LastHandoff.FromAgentID, valueOrDash(session.LastHandoff.ToAgentID))
}

func renderList(result middleware.SessionActionResult, lang string, deps RenderDeps) string {
	if result.Message != "" && len(result.Sessions) == 0 && len(result.RemoteSessions) == 0 {
		return result.Message
	}
	var sb strings.Builder
	writeLocalSessions(&sb, result.Sessions, lang, deps)
	writeRemoteSessions(&sb, result.RemoteSessions, deps)
	if sb.Len() == 0 {
		return result.Message
	}
	return sb.String()
}

func writeLocalSessions(sb *strings.Builder, sessions []middleware.SessionEntry, lang string, deps RenderDeps) {
	if len(sessions) == 0 {
		return
	}
	sb.WriteString(deps.Lookup("session_history_header") + "\n")
	for i, session := range sessions {
		sb.WriteString(deps.Local(i, lang, session))
	}
}

func writeRemoteSessions(sb *strings.Builder, sessions []middleware.RemoteSessionInfo, deps RenderDeps) {
	if len(sessions) == 0 {
		return
	}
	if sb.Len() > 0 {
		sb.WriteString("\n")
	}
	sb.WriteString("Remote sessions:\n")
	for i, session := range sessions {
		sb.WriteString(deps.Remote(i, session))
	}
}

func valueOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
