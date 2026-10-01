package session

import (
	"strings"

	"github.com/Josepavese/matrix/internal/logic/workspace"
)

// Route kinds produced by planWorkspaceRoute. They name the decision, not an
// agent or a channel: any agent routed with the same workspace hints takes the
// same branch.
const (
	workspaceRouteReuseActive   = "reuse-active-session"
	workspaceRouteResumeIndexed = "resume-workspace-session"
	workspaceRouteCreate        = "create-session"
)

// SessionAffinityPlan is the read-only prediction of the session a request will
// use: which logical session, which remote session id it will hand back to the
// provider, and the workspace both belong to. Callers publish it before the
// prompt so a reused remote session is never silent.
type SessionAffinityPlan struct {
	Kind                string
	LogicalSessionID    string
	RemoteSessionID     string
	AgentID             string
	WorkspaceID         string
	WorkspacePath       string
	NewSession          bool
	ReusesRemoteSession bool
}

type workspaceRoutePlan struct {
	kind      string
	request   workspaceRouteRequest
	sessionID string
	meta      SessionMeta
}

func newWorkspaceRouteRequest(channelID, targetAgent, workspaceID, workspacePath string) workspaceRouteRequest {
	return workspaceRouteRequest{
		ChannelID:     channelID,
		TargetAgent:   strings.TrimSpace(targetAgent),
		WorkspaceID:   strings.TrimSpace(workspaceID),
		WorkspacePath: strings.TrimSpace(workspacePath),
	}
}

// planWorkspaceRoute resolves the routing decision without mutating state. It
// mirrors getOrCreateSessionForWorkspace step for step — channel-active reuse,
// then the workspace session index, then the channel's preferred workspace,
// then creation — so the published plan and the routing that follows agree.
func (m *Manager) planWorkspaceRoute(req workspaceRouteRequest) (workspaceRoutePlan, error) {
	// At most two attempts: the second one only happens when the channel's
	// preferred workspace is substituted for a request that carried no id, and
	// the substitution cannot repeat.
	for attempt := 0; attempt < 2; attempt++ {
		state, stateErr := m.getChannelState(req.ChannelID)
		if plan, found := m.activeWorkspaceRoutePlan(req, state, stateErr); found {
			return plan, nil
		}
		if plan, found, err := m.indexedWorkspaceRoutePlan(req); err != nil || found {
			return plan, err
		}
		next, substituted, err := m.substitutePreferredWorkspace(req, state, stateErr)
		if err != nil {
			return workspaceRoutePlan{}, err
		}
		if !substituted {
			return workspaceRoutePlan{kind: workspaceRouteCreate, request: req}, nil
		}
		req = next
	}
	return workspaceRoutePlan{kind: workspaceRouteCreate, request: req}, nil
}

// activeWorkspaceRoutePlan reuses the channel's active session when it already
// matches the requested workspace and agent.
func (m *Manager) activeWorkspaceRoutePlan(req workspaceRouteRequest, state ChannelState, stateErr error) (workspaceRoutePlan, bool) {
	if !canReuseActiveState(state, stateErr) {
		return workspaceRoutePlan{}, false
	}
	meta, found := m.loadReusableSessionMeta(state.ActiveSessionID)
	if !found || !workspaceRouteMatches(meta, req) {
		return workspaceRoutePlan{}, false
	}
	return workspaceRoutePlan{kind: workspaceRouteReuseActive, request: req, sessionID: state.ActiveSessionID, meta: meta}, true
}

// indexedWorkspaceRoutePlan resumes a session the workspace index offers for the
// requested id.
func (m *Manager) indexedWorkspaceRoutePlan(req workspaceRouteRequest) (workspaceRoutePlan, bool, error) {
	if req.WorkspaceID == "" {
		return workspaceRoutePlan{}, false, nil
	}
	candidate, found, err := m.indexedWorkspaceSessionCandidate(req)
	if err != nil || !found {
		return workspaceRoutePlan{}, false, err
	}
	return workspaceRoutePlan{kind: workspaceRouteResumeIndexed, request: req, sessionID: candidate.SessionID, meta: candidate.Meta}, true, nil
}

// substitutePreferredWorkspace falls back to the channel's preferred workspace
// when the request carried no id. The substituted id still has to agree with the
// requested path: binding one workspace id to another workspace's directory is
// the ambiguous association this contract refuses.
func (m *Manager) substitutePreferredWorkspace(req workspaceRouteRequest, state ChannelState, stateErr error) (workspaceRouteRequest, bool, error) {
	if !canSubstitutePreferredWorkspace(req, state, stateErr) {
		return req, false, nil
	}
	req.WorkspaceID = state.PreferredWorkspaceID
	if _, _, err := m.resolveWorkspaceHint(req.WorkspaceID, req.WorkspacePath); err != nil {
		return req, false, err
	}
	return req, true, nil
}

// canSubstitutePreferredWorkspace is true only when the request carried no
// workspace id and the channel actually names a preferred one. An unreadable
// channel state is not a routing failure: routing falls through to creating a
// session, and so does the plan.
func canSubstitutePreferredWorkspace(req workspaceRouteRequest, state ChannelState, stateErr error) bool {
	return req.WorkspaceID == "" && stateErr == nil && strings.TrimSpace(state.PreferredWorkspaceID) != ""
}

// indexedWorkspaceSessionCandidate returns the session the workspace index
// offers for these hints, skipping every candidate whose recorded workspace
// affinity does not match the request.
func (m *Manager) indexedWorkspaceSessionCandidate(req workspaceRouteRequest) (workspaceSessionCandidate, bool, error) {
	sessionIDs, err := workspace.LoadSessionIndex(m.storage, req.WorkspaceID)
	if err != nil {
		return workspaceSessionCandidate{}, false, err
	}
	for _, sessionID := range sessionIDs {
		candidate, ok := m.workspaceSessionCandidate(req, sessionID)
		if !ok {
			continue
		}
		if req.TargetAgent != "" && candidate.Meta.AgentID != req.TargetAgent {
			continue
		}
		return candidate, true, nil
	}
	return workspaceSessionCandidate{}, false, nil
}

// PlanSessionAffinity predicts the session a request with these hints would use
// without creating or resuming anything. It is the pre-prompt evidence that a
// new channel did not silently adopt another workstream's remote session.
func (m *Manager) PlanSessionAffinity(channelID, agentID, workspaceID, workspacePath string) (SessionAffinityPlan, error) {
	resolvedID, resolvedPath, err := m.resolveWorkspaceHint(workspaceID, workspacePath)
	if err != nil {
		return SessionAffinityPlan{}, err
	}
	plan, err := m.planWorkspaceRoute(newWorkspaceRouteRequest(channelID, agentID, resolvedID, resolvedPath))
	if err != nil {
		return SessionAffinityPlan{}, err
	}
	affinity := SessionAffinityPlan{
		Kind:          plan.kind,
		WorkspaceID:   firstNonEmpty(plan.request.WorkspaceID, resolvedID),
		WorkspacePath: firstNonEmpty(plan.request.WorkspacePath, resolvedPath),
		NewSession:    plan.kind == workspaceRouteCreate,
	}
	if plan.sessionID != "" {
		affinity.LogicalSessionID = plan.sessionID
		affinity.RemoteSessionID = strings.TrimSpace(plan.meta.AgentSessionID)
		affinity.AgentID = strings.TrimSpace(plan.meta.AgentID)
	}
	if affinity.AgentID == "" {
		affinity.AgentID = firstNonEmpty(strings.TrimSpace(agentID), m.defaultAgent)
	}
	affinity.ReusesRemoteSession = affinity.RemoteSessionID != ""
	return affinity, nil
}
