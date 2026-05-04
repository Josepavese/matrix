package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/workspace"
	"github.com/Josepavese/matrix/internal/middleware"
)

func (m *Manager) handleWorkspaceStateReadTyped(_ context.Context, channelID, _, workspaceID string) (middleware.WorkspaceReadResult, error) {
	ws, meta, err := m.resolveWorkspaceReadContext(channelID, workspaceID)
	if err != nil {
		return middleware.WorkspaceReadResult{}, err
	}
	state, found, err := workspace.LoadState(m.storage, ws.ID)
	if err != nil {
		return middleware.WorkspaceReadResult{}, err
	}
	if !found {
		state = workspace.State{WorkspaceID: ws.ID}
	}
	entry := m.toWorkspaceStateEntry(state)
	msg := renderWorkspaceState(entry)
	var sessionEntry *middleware.SessionEntry
	if meta.ID != "" {
		sessionEntry = m.toSessionEntry(meta, true)
	}
	return middleware.WorkspaceReadResult{
		Action:    "state",
		Message:   msg,
		Workspace: workspaceEntryPtr(m.toWorkspaceEntry(ws, true)),
		State:     &entry,
		Session:   sessionEntry,
	}, nil
}

func (m *Manager) handleWorkspaceTimelineReadTyped(_ context.Context, channelID, _, workspaceID string, limit int) (middleware.WorkspaceReadResult, error) {
	ws, meta, err := m.resolveWorkspaceReadContext(channelID, workspaceID)
	if err != nil {
		return middleware.WorkspaceReadResult{}, err
	}
	if limit <= 0 {
		limit = 10
	}
	events, err := workspace.LoadTimeline(m.storage, ws.ID, limit)
	if err != nil {
		return middleware.WorkspaceReadResult{}, err
	}
	timeline := make([]middleware.WorkspaceTimelineEvent, 0, len(events))
	for _, event := range events {
		timeline = append(timeline, m.toWorkspaceTimelineEvent(event))
	}
	msg := renderWorkspaceTimeline(ws.ID, timeline)
	var sessionEntry *middleware.SessionEntry
	if meta.ID != "" {
		sessionEntry = m.toSessionEntry(meta, true)
	}
	return middleware.WorkspaceReadResult{
		Action:    "timeline",
		Message:   msg,
		Workspace: workspaceEntryPtr(m.toWorkspaceEntry(ws, true)),
		Timeline:  timeline,
		Session:   sessionEntry,
	}, nil
}

func (m *Manager) handleWorkspaceDecisionsReadTyped(_ context.Context, channelID, _, workspaceID string, limit int) (middleware.WorkspaceReadResult, error) {
	ws, meta, err := m.resolveWorkspaceReadContext(channelID, workspaceID)
	if err != nil {
		return middleware.WorkspaceReadResult{}, err
	}
	if limit <= 0 {
		limit = 10
	}
	events, err := workspace.LoadTimeline(m.storage, ws.ID, 100)
	if err != nil {
		return middleware.WorkspaceReadResult{}, err
	}
	decisions := make([]middleware.WorkspaceDecisionTrace, 0, limit)
	for _, event := range events {
		if trace := decisionTraceFromEvent(event); trace != nil {
			decisions = append(decisions, *trace)
			if len(decisions) >= limit {
				break
			}
		}
	}
	msg := renderWorkspaceDecisions(ws.ID, decisions)
	var sessionEntry *middleware.SessionEntry
	if meta.ID != "" {
		sessionEntry = m.toSessionEntry(meta, true)
	}
	return middleware.WorkspaceReadResult{
		Action:    "decisions",
		Message:   msg,
		Workspace: workspaceEntryPtr(m.toWorkspaceEntry(ws, true)),
		Session:   sessionEntry,
		Decisions: decisions,
	}, nil
}

func (m *Manager) handleWorkspaceMemoryReadTyped(_ context.Context, channelID, _, workspaceID string, limit int) (middleware.WorkspaceReadResult, error) {
	ws, meta, err := m.resolveWorkspaceReadContext(channelID, workspaceID)
	if err != nil {
		return middleware.WorkspaceReadResult{}, err
	}
	if limit <= 0 {
		limit = 12
	}
	turns, err := workspace.LoadTurns(m.storage, ws.ID, limit)
	if err != nil {
		return middleware.WorkspaceReadResult{}, err
	}
	memory := make([]middleware.WorkspaceMemoryTurn, 0, len(turns))
	for _, turn := range turns {
		memory = append(memory, m.toWorkspaceMemoryTurn(turn))
	}
	msg := renderWorkspaceMemory(ws.ID, memory)
	var sessionEntry *middleware.SessionEntry
	if meta.ID != "" {
		sessionEntry = m.toSessionEntry(meta, true)
	}
	return middleware.WorkspaceReadResult{
		Action:    "memory",
		Message:   msg,
		Workspace: workspaceEntryPtr(m.toWorkspaceEntry(ws, true)),
		Session:   sessionEntry,
		Memory:    memory,
	}, nil
}

func (m *Manager) handleWorkspaceSnapshotsReadTyped(_ context.Context, channelID, _, workspaceID string, limit int) (middleware.WorkspaceReadResult, error) {
	ws, meta, err := m.resolveWorkspaceReadContext(channelID, workspaceID)
	if err != nil {
		return middleware.WorkspaceReadResult{}, err
	}
	if limit <= 0 {
		limit = 10
	}
	snapshots, err := workspace.LoadSnapshots(m.storage, ws.ID, limit)
	if err != nil {
		return middleware.WorkspaceReadResult{}, err
	}
	entries := make([]middleware.WorkspaceSnapshotEntry, 0, len(snapshots))
	for _, snapshot := range snapshots {
		entries = append(entries, m.toWorkspaceSnapshotEntry(snapshot))
	}
	msg := renderWorkspaceSnapshots(ws.ID, entries)
	var sessionEntry *middleware.SessionEntry
	if meta.ID != "" {
		sessionEntry = m.toSessionEntry(meta, true)
	}
	return middleware.WorkspaceReadResult{
		Action:    "snapshots",
		Message:   msg,
		Workspace: workspaceEntryPtr(m.toWorkspaceEntry(ws, true)),
		Session:   sessionEntry,
		Snapshots: entries,
	}, nil
}

func (m *Manager) resolveWorkspaceReadContext(channelID, requestedWorkspaceID string) (workspace.Meta, SessionMeta, error) {
	if strings.TrimSpace(requestedWorkspaceID) != "" {
		ws, err := m.loadRequiredWorkspace(requestedWorkspaceID)
		if err != nil {
			return workspace.Meta{}, SessionMeta{}, err
		}
		meta, _ := m.currentSessionForWorkspace(channelID, ws.ID)
		return ws, meta, nil
	}
	state, _ := m.getChannelState(channelID)
	if ws, found, err := m.workspaceFromPreferredState(state); err != nil || found {
		meta, _ := m.currentSessionForWorkspace(channelID, ws.ID)
		return ws, meta, err
	}
	if ws, meta, found, err := m.workspaceFromActiveSessionState(state); err != nil || found {
		return ws, meta, err
	}
	return workspace.Meta{}, SessionMeta{}, fmt.Errorf("no workspace context available")
}

func (m *Manager) toWorkspaceStateEntry(state workspace.State) middleware.WorkspaceStateEntry {
	entry := middleware.WorkspaceStateEntry{
		WorkspaceID:            state.WorkspaceID,
		ActiveLogicalSessionID: state.ActiveLogicalSessionID,
		ActiveRemoteSessionID:  state.ActiveRemoteSessionID,
		ActiveAgentID:          state.ActiveAgentID,
		ActiveMode:             state.ActiveMode,
		RemoteStatus:           state.RemoteStatus,
		LastEventType:          state.LastEventType,
		LastEventMessage:       state.LastEventMessage,
		LastHandoff:            state.LastHandoff,
		LastDecision:           toDecisionTrace(state.LastDecision),
	}
	if !state.LastEventAt.IsZero() {
		entry.LastEventAt = state.LastEventAt.Format(time.RFC3339)
	}
	return entry
}

func (m *Manager) toWorkspaceTimelineEvent(event workspace.Event) middleware.WorkspaceTimelineEvent {
	return middleware.WorkspaceTimelineEvent{
		ID:               event.ID,
		WorkspaceID:      event.WorkspaceID,
		Type:             event.Type,
		ChannelID:        event.ChannelID,
		LogicalSessionID: event.LogicalSessionID,
		RemoteSessionID:  event.RemoteSessionID,
		AgentID:          event.AgentID,
		Mode:             event.Mode,
		Message:          event.Message,
		Reason:           event.Reason,
		Metadata:         event.Metadata,
		CreatedAt:        event.CreatedAt.Format(time.RFC3339),
	}
}

func (m *Manager) toWorkspaceMemoryTurn(turn workspace.Turn) middleware.WorkspaceMemoryTurn {
	return middleware.WorkspaceMemoryTurn{
		ID:               turn.ID,
		WorkspaceID:      turn.WorkspaceID,
		LogicalSessionID: turn.LogicalSessionID,
		RemoteSessionID:  turn.RemoteSessionID,
		AgentID:          turn.AgentID,
		Role:             turn.Role,
		Content:          turn.Content,
		CreatedAt:        turn.CreatedAt.Format(time.RFC3339),
	}
}

func (m *Manager) toWorkspaceSnapshotEntry(snapshot workspace.Snapshot) middleware.WorkspaceSnapshotEntry {
	entry := middleware.WorkspaceSnapshotEntry{
		ID:                     snapshot.ID,
		WorkspaceID:            snapshot.WorkspaceID,
		Title:                  snapshot.Title,
		Note:                   snapshot.Note,
		ActiveLogicalSessionID: snapshot.ActiveLogicalSessionID,
		ActiveRemoteSessionID:  snapshot.ActiveRemoteSessionID,
		ActiveAgentID:          snapshot.ActiveAgentID,
		ActiveMode:             snapshot.ActiveMode,
		RemoteStatus:           snapshot.RemoteStatus,
		LastEventType:          snapshot.LastEventType,
		LastHandoff:            snapshot.LastHandoff,
		LastDecision:           toDecisionTrace(snapshot.LastDecision),
		TurnIDs:                snapshot.TurnIDs,
		EventIDs:               snapshot.EventIDs,
		CreatedAt:              snapshot.CreatedAt.Format(time.RFC3339),
	}
	if !snapshot.LastEventAt.IsZero() {
		entry.LastEventAt = snapshot.LastEventAt.Format(time.RFC3339)
	}
	return entry
}

func renderWorkspaceState(state middleware.WorkspaceStateEntry) string {
	lines := []string{
		fmt.Sprintf("Workspace: %s", valueOrDash(state.WorkspaceID)),
		fmt.Sprintf("Mode: %s", valueOrDash(state.ActiveMode)),
		fmt.Sprintf("Agent: %s", valueOrDash(state.ActiveAgentID)),
		fmt.Sprintf("Session: %s", shortOrDash(state.ActiveLogicalSessionID, 8)),
		fmt.Sprintf("Remote status: %s", valueOrDash(state.RemoteStatus)),
		fmt.Sprintf("Last event: %s", describeWorkspaceEventType(state.LastEventType)),
	}
	if state.LastEventMessage != "" {
		lines = append(lines, fmt.Sprintf("Event detail: %s", state.LastEventMessage))
	}
	if state.LastEventAt != "" {
		lines = append(lines, fmt.Sprintf("Updated: %s", formatTimestamp(state.LastEventAt)))
	}
	if state.LastHandoff != nil {
		handoff := fmt.Sprintf("Handoff: %s -> %s", valueOrDash(stringify(state.LastHandoff["from_agent_id"])), valueOrDash(stringify(state.LastHandoff["to_agent_id"])))
		if summary := stringify(state.LastHandoff["summary"]); strings.TrimSpace(summary) != "" {
			handoff += " - " + summary
		}
		lines = append(lines, handoff)
	}
	if state.LastDecision != nil {
		lines = append(lines, "Decision: "+describeDecisionTrace(state.LastDecision))
	}
	return strings.Join(lines, "\n")
}

func renderWorkspaceTimeline(workspaceID string, timeline []middleware.WorkspaceTimelineEvent) string {
	if len(timeline) == 0 {
		return fmt.Sprintf("Workspace %s has no timeline events yet.", workspaceID)
	}
	lines := make([]string, 0, len(timeline)+1)
	lines = append(lines, fmt.Sprintf("Workspace timeline: %s", workspaceID))
	for idx, event := range timeline {
		line := fmt.Sprintf("[%d] %s", idx+1, describeTimelineEvent(event))
		if timestamp := formatTimestamp(event.CreatedAt); timestamp != "-" {
			line += " [" + timestamp + "]"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderWorkspaceDecisions(workspaceID string, decisions []middleware.WorkspaceDecisionTrace) string {
	if len(decisions) == 0 {
		return fmt.Sprintf("Workspace %s has no recorded orchestration decisions yet.", workspaceID)
	}
	lines := []string{fmt.Sprintf("Workspace decisions: %s", workspaceID)}
	for idx, decision := range decisions {
		line := fmt.Sprintf("[%d] %s", idx+1, describeDecisionTrace(&decision))
		if timestamp := formatTimestamp(decision.CreatedAt); timestamp != "-" {
			line += " [" + timestamp + "]"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderWorkspaceMemory(workspaceID string, memory []middleware.WorkspaceMemoryTurn) string {
	if len(memory) == 0 {
		return fmt.Sprintf("Workspace %s has no stored work memory yet.", workspaceID)
	}
	lines := []string{fmt.Sprintf("Workspace memory: %s", workspaceID)}
	for idx, turn := range memory {
		role := turn.Role
		if role == "" {
			role = "turn"
		}
		lines = append(lines, fmt.Sprintf("[%d] %s - %s [%s]", idx+1, role, trimForDisplay(turn.Content, 140), formatTimestamp(turn.CreatedAt)))
	}
	return strings.Join(lines, "\n")
}

func renderWorkspaceSnapshots(workspaceID string, snapshots []middleware.WorkspaceSnapshotEntry) string {
	if len(snapshots) == 0 {
		return fmt.Sprintf("Workspace %s has no snapshots yet.", workspaceID)
	}
	lines := []string{fmt.Sprintf("Workspace snapshots: %s", workspaceID)}
	for idx, snapshot := range snapshots {
		title := snapshot.Title
		if strings.TrimSpace(title) == "" {
			title = "workspace snapshot"
		}
		line := fmt.Sprintf("[%d] %s - %s", idx+1, shortOrDash(snapshot.ID, 8), title)
		if snapshot.ActiveAgentID != "" || snapshot.ActiveMode != "" {
			line += fmt.Sprintf(" (%s, %s)", valueOrDash(snapshot.ActiveAgentID), valueOrDash(snapshot.ActiveMode))
		}
		if strings.TrimSpace(snapshot.Note) != "" {
			line += " - " + snapshot.Note
		}
		line += " [" + formatTimestamp(snapshot.CreatedAt) + "]"
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func describeTimelineEvent(event middleware.WorkspaceTimelineEvent) string {
	if handler := timelineEventHandlers[event.Type]; handler != nil {
		if description := handler(event); description != "" {
			return description
		}
	}

	parts := []string{describeWorkspaceEventType(event.Type)}
	if event.AgentID != "" {
		parts = append(parts, event.AgentID)
	}
	if event.Mode != "" {
		parts = append(parts, event.Mode)
	}
	if event.Message != "" {
		parts = append(parts, event.Message)
	}
	return strings.Join(parts, " - ")
}

func describeWorkspaceEventType(eventType string) string {
	if description := workspaceEventTypeDescriptions[eventType]; description != "" {
		return description
	}
	if strings.TrimSpace(eventType) == "" {
		return "-"
	}
	return eventType
}

var timelineEventHandlers = map[string]func(middleware.WorkspaceTimelineEvent) string{
	"handoff.created":    describeHandoffCreatedEvent,
	"handoff.applied":    describeHandoffAppliedEvent,
	"decision.recorded":  describeDecisionRecordedEvent,
	"mode.changed":       describeModeChangedEvent,
	"intent.review":      staticTimelineDescription("entered review mode"),
	"intent.explain":     staticTimelineDescription("entered explain mode"),
	"intent.triage":      staticTimelineDescription("entered triage mode"),
	"intent.resume":      staticTimelineDescription("resumed workspace context"),
	"intent.continue":    staticTimelineDescription("continued current work"),
	"intent.handoff":     describeAgentTargetedEvent("handed work to "),
	"session.created":    describeAgentTargetedEvent("created session for "),
	"session.resumed":    describeAgentTargetedEvent("resumed session for "),
	"session.switched":   describeAgentTargetedEvent("switched to session for "),
	"session.canceled":   staticTimelineDescription("canceled remote session"),
	"session.deleted":    staticTimelineDescription("deleted session"),
	"workspace.bound":    staticTimelineDescription("bound session to workspace"),
	"workspace.switched": staticTimelineDescription("switched workspace context"),
}

func describeHandoffCreatedEvent(event middleware.WorkspaceTimelineEvent) string {
	fromAgent := valueOrDash(stringify(event.Metadata["from_agent_id"]))
	toAgent := valueOrDash(stringify(event.Metadata["to_agent_id"]))
	if summary := strings.TrimSpace(stringify(event.Metadata["summary"])); summary != "" {
		return fmt.Sprintf("handoff created %s -> %s - %s", fromAgent, toAgent, summary)
	}
	return fmt.Sprintf("handoff created %s -> %s", fromAgent, toAgent)
}

func describeHandoffAppliedEvent(event middleware.WorkspaceTimelineEvent) string {
	fromAgent := valueOrDash(stringify(event.Metadata["from_agent_id"]))
	toAgent := valueOrDash(stringify(event.Metadata["to_agent_id"]))
	return fmt.Sprintf("handoff applied %s -> %s", fromAgent, toAgent)
}

func describeDecisionRecordedEvent(event middleware.WorkspaceTimelineEvent) string {
	trace := toDecisionTrace(event.Metadata)
	if trace == nil {
		return ""
	}
	if trace.CreatedAt == "" {
		trace.CreatedAt = event.CreatedAt
	}
	return describeDecisionTrace(trace)
}

func describeModeChangedEvent(event middleware.WorkspaceTimelineEvent) string {
	if event.Mode == "" {
		return ""
	}
	return "mode changed to " + event.Mode
}

func staticTimelineDescription(description string) func(middleware.WorkspaceTimelineEvent) string {
	return func(middleware.WorkspaceTimelineEvent) string {
		return description
	}
}

func describeAgentTargetedEvent(prefix string) func(middleware.WorkspaceTimelineEvent) string {
	return func(event middleware.WorkspaceTimelineEvent) string {
		if event.AgentID == "" {
			return ""
		}
		return prefix + event.AgentID
	}
}

var workspaceEventTypeDescriptions = map[string]string{
	"session.created":    "session created",
	"session.resumed":    "session resumed",
	"session.switched":   "session switched",
	"session.canceled":   "session canceled",
	"session.deleted":    "session deleted",
	"workspace.bound":    "workspace bound",
	"workspace.switched": "workspace switched",
	"mode.changed":       "mode changed",
	"intent.continue":    "continue intent",
	"intent.resume":      "resume intent",
	"intent.review":      "review intent",
	"intent.explain":     "explain intent",
	"intent.triage":      "triage intent",
	"intent.handoff":     "handoff intent",
	"handoff.created":    "handoff created",
	"handoff.applied":    "handoff applied",
	"decision.recorded":  "decision recorded",
}

func formatTimestamp(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format("2006-01-02 15:04 UTC")
}

func trimForDisplay(value string, maxLen int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if maxLen > 0 && len(value) > maxLen {
		return value[:maxLen-3] + "..."
	}
	return value
}

func shortOrDash(value string, limit int) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	if limit > 0 && len(value) > limit {
		return value[:limit]
	}
	return value
}

func stringify(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
