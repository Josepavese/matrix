package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/sessioncleanup"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/google/uuid"
)

type forkPlan struct {
	State         ChannelState
	Parent        SessionMeta
	ChildRemote   middleware.RemoteSessionInfo
	MakeActive    bool
	RestoreParent bool
	CleanupPolicy string
}

type forkResultData struct {
	ChildMeta      SessionMeta
	Session        *middleware.SessionEntry
	Plan           forkPlan
	ActiveID       string
	ParentRestored bool
	Async          bool
	Job            *middleware.SessionForkJob
	Artifact       *middleware.SessionForkArtifact
	Cleanup        *middleware.SessionCleanupResult
}

//nolint:nilerr // Fork failures are protocol-visible typed results, not Go transport errors.
func (m *Manager) handleSessionForkTyped(ctx context.Context, req middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	plan, unsupported, err := m.prepareFork(ctx, req)
	if unsupported != nil || err != nil {
		if unsupported != nil {
			return *unsupported, nil
		}
		return middleware.SessionActionResult{}, err
	}
	childMeta := buildForkChildMeta(plan.Parent, plan.ChildRemote, req.Ephemeral, plan.CleanupPolicy)
	if err := m.persistForkChild(req.ChannelID, childMeta, plan.MakeActive); err != nil {
		return middleware.SessionActionResult{}, err
	}
	if forkAsyncRequested(req) {
		return m.acceptAsyncFork(ctx, req, plan, childMeta)
	}
	artifact, cleanup, err := m.runForkChildWorkflow(ctx, req, childMeta, plan.CleanupPolicy)
	if err != nil {
		return m.forkWorkflowFailedResult(forkWorkflowFailureData{
			ChannelID: req.ChannelID,
			Plan:      plan,
			ChildMeta: childMeta,
			Artifact:  artifact,
			Cleanup:   cleanup,
			Err:       err,
		}), nil
	}
	activeID, parentRestored, err := m.restoreForkParent(req.ChannelID, plan, childMeta.ID)
	if err != nil {
		return m.forkWorkflowFailedResult(forkWorkflowFailureData{
			ChannelID: req.ChannelID,
			Plan:      plan,
			ChildMeta: childMeta,
			Artifact:  artifact,
			Cleanup:   cleanup,
			Err: forkWorkflowError{
				code:    "fork_parent_restore_failed",
				message: "failed to restore parent session after fork",
				err:     err,
			},
		}), nil
	}
	childActive := activeID == childMeta.ID
	session := m.forkSessionEntry(childMeta, childActive, cleanup)
	return forkActionResult(forkResultData{
		ChildMeta:      childMeta,
		Session:        session,
		Plan:           plan,
		ActiveID:       activeID,
		ParentRestored: parentRestored,
		Artifact:       artifact,
		Cleanup:        cleanup,
	}), nil
}

func (m *Manager) prepareFork(ctx context.Context, req middleware.SessionActionRequest) (forkPlan, *middleware.SessionActionResult, error) {
	state, err := m.getChannelState(req.ChannelID)
	if err != nil {
		return forkPlan{}, nil, err
	}
	meta, err := m.resolveActionSession(req)
	if err != nil {
		return forkPlan{}, nil, err
	}
	forker, unsupported := m.sessionForker(meta)
	if unsupported != nil {
		return forkPlan{}, unsupported, nil
	}
	meta, unsupported, err = m.ensureForkParentRemote(ctx, req, meta)
	if unsupported != nil || err != nil {
		return forkPlan{}, unsupported, err
	}
	child, err := forker.ForkAgentSession(ctx, meta.AgentID, middleware.SessionForkRequest{
		RemoteSessionID:       meta.AgentSessionID,
		WorkspacePath:         firstNonEmpty(req.WorkspacePath, meta.WorkspacePath),
		AdditionalDirectories: req.AdditionalDirectories,
	})
	if err != nil {
		result := unsupportedForkResult(meta, err.Error())
		return forkPlan{}, &result, nil
	}
	makeActive := forkMakeActive(req)
	return forkPlan{
		State:         state,
		Parent:        meta,
		ChildRemote:   child,
		MakeActive:    makeActive,
		RestoreParent: req.RestoreParent || !makeActive || strings.TrimSpace(req.Input) != "",
		CleanupPolicy: forkCleanupPolicy(req),
	}, nil, nil
}

func (m *Manager) sessionForker(meta SessionMeta) (middleware.AgentSessionForker, *middleware.SessionActionResult) {
	forker, ok := m.router.(middleware.AgentSessionForker)
	if ok {
		return forker, nil
	}
	result := unsupportedForkResult(meta, "router does not expose session fork")
	return nil, &result
}

func (m *Manager) ensureForkParentRemote(ctx context.Context, req middleware.SessionActionRequest, meta SessionMeta) (SessionMeta, *middleware.SessionActionResult, error) {
	if strings.TrimSpace(meta.AgentSessionID) != "" {
		return meta, nil, nil
	}
	if result := m.forkUnsupportedByCapabilities(ctx, meta); result != nil {
		return SessionMeta{}, result, nil
	}
	return m.materializeForkParent(ctx, req, meta)
}

func forkCleanupPolicy(req middleware.SessionActionRequest) string {
	if req.Ephemeral || strings.TrimSpace(req.CleanupPolicy) != "" {
		return sessioncleanup.NormalizePolicy(req.CleanupPolicy)
	}
	return ""
}

func buildForkChildMeta(parent SessionMeta, child middleware.RemoteSessionInfo, ephemeral bool, cleanupPolicy string) SessionMeta {
	now := time.Now().UTC()
	childMeta := parent
	childMeta.ID = uuid.New().String()
	childMeta.AgentSessionID = child.RemoteSessionID
	childMeta.CreatedAt = now
	childMeta.Alias = ""
	childMeta.ParentSessionID = parent.ID
	childMeta.ParentRemoteID = parent.AgentSessionID
	childMeta.Ephemeral = ephemeral
	childMeta.CleanupPolicy = cleanupPolicy
	childMeta.RemoteTitle = child.Title
	childMeta.RemoteStatus = child.Status
	childMeta.LastSyncedAt = now
	childMeta.RemoteUpdatedAt = time.Time{}
	childMeta.PendingHandoff = nil
	childMeta.LastHandoff = nil
	if child.UpdatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, child.UpdatedAt); err == nil {
			childMeta.RemoteUpdatedAt = parsed
		}
	}
	return childMeta
}

func (m *Manager) persistForkChild(channelID string, childMeta SessionMeta, makeActive bool) error {
	if err := m.saveSessionMeta(childMeta); err != nil {
		return err
	}
	if makeActive {
		if err := m.updateChannelState(channelID, childMeta.ID); err != nil {
			return err
		}
	} else if err := m.appendInactiveChannelSession(channelID, childMeta.ID); err != nil {
		return err
	}
	return m.indexSessionWorkspace(childMeta)
}

func (m *Manager) restoreForkParent(channelID string, plan forkPlan, childID string) (string, bool, error) {
	activeID := childID
	parentRestored := false
	if plan.RestoreParent {
		restoreID := firstNonEmpty(plan.Parent.ID, plan.State.ActiveSessionID)
		if restoreID == "" {
			return activeID, false, nil
		}
		if found, err := m.sessionMetaExists(restoreID); err != nil || !found {
			if err != nil {
				return "", false, err
			}
			state, stateErr := m.getChannelState(channelID)
			if stateErr != nil {
				return activeID, false, stateErr
			}
			return state.ActiveSessionID, false, nil
		}
		if err := m.updateChannelState(channelID, restoreID); err != nil {
			return "", false, err
		}
		return restoreID, true, nil
	}
	if !plan.MakeActive {
		activeID = plan.State.ActiveSessionID
		parentRestored = plan.State.ActiveSessionID == plan.Parent.ID
	}
	return activeID, parentRestored, nil
}

func (m *Manager) sessionMetaExists(sessionID string) (bool, error) {
	_, found, err := m.loadSessionMeta(sessionID)
	return found, err
}

func forkActionResult(data forkResultData) middleware.SessionActionResult {
	message := fmt.Sprintf("Forked session: %s", data.ChildMeta.ID)
	if data.Async && data.Job != nil {
		message = fmt.Sprintf("Fork accepted asynchronously: %s", data.Job.JobID)
	}
	return middleware.SessionActionResult{
		Action:          "fork",
		Message:         message,
		ActiveSessionID: data.ActiveID,
		Session:         data.Session,
		Fork: &middleware.SessionForkResult{
			ParentLogicalSessionID: data.Plan.Parent.ID,
			ParentRemoteSessionID:  data.Plan.Parent.AgentSessionID,
			ChildLogicalSessionID:  data.ChildMeta.ID,
			Child:                  &data.Plan.ChildRemote,
			MakeActive:             data.Plan.MakeActive,
			Ephemeral:              data.ChildMeta.Ephemeral,
			CleanupPolicy:          data.Plan.CleanupPolicy,
			Async:                  data.Async,
			JobID:                  forkJobID(data.Job),
			Job:                    data.Job,
			Artifact:               data.Artifact,
			Cleanup:                data.Cleanup,
			ParentRestored:         data.ParentRestored,
		},
	}
}

func (m *Manager) forkSessionEntry(meta SessionMeta, active bool, cleanup *middleware.SessionCleanupResult) *middleware.SessionEntry {
	if cleanup != nil && cleanup.LocalForgotten {
		return nil
	}
	return m.toSessionEntry(meta, active)
}

func unsupportedForkResult(meta SessionMeta, reason string) middleware.SessionActionResult {
	return middleware.SessionActionResult{
		Action:      "fork",
		Message:     "Session fork is unsupported by this provider.",
		Unsupported: true,
		Fork: &middleware.SessionForkResult{
			ParentLogicalSessionID: meta.ID,
			ParentRemoteSessionID:  meta.AgentSessionID,
			Unsupported:            true,
			Reason:                 reason,
		},
	}
}

func forkMakeActive(req middleware.SessionActionRequest) bool {
	if req.MakeActive != nil {
		return *req.MakeActive
	}
	return strings.TrimSpace(req.Input) == ""
}

func forkAsyncRequested(req middleware.SessionActionRequest) bool {
	return req.Async && strings.TrimSpace(req.Input) != ""
}

func forkJobID(job *middleware.SessionForkJob) string {
	if job == nil {
		return ""
	}
	return job.JobID
}
