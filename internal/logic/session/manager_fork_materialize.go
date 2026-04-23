package session

import (
	"context"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
)

func (m *Manager) forkUnsupportedByCapabilities(ctx context.Context, meta SessionMeta) *middleware.SessionActionResult {
	reporter, ok := m.router.(middleware.AgentCapabilityReporter)
	if !ok {
		return nil
	}
	report, err := reporter.AgentCapabilities(ctx, meta.AgentID)
	if err != nil {
		return nil
	}
	fork, ok := report.Session["fork"]
	if !ok || fork.Supported {
		return nil
	}
	result := unsupportedForkResult(meta, firstNonEmpty(fork.Detail, "provider does not advertise session/fork"))
	return &result
}

func (m *Manager) materializeForkParent(ctx context.Context, req middleware.SessionActionRequest, meta SessionMeta) (SessionMeta, *middleware.SessionActionResult, error) {
	materializer, ok := m.router.(middleware.AgentSessionMaterializer)
	if !ok {
		result := missingRemoteForkResult(meta, "router does not expose remote session materialization")
		return SessionMeta{}, &result, nil
	}
	remote, metadata, err := materializer.MaterializeAgentSession(ctx, meta.AgentID, middleware.SessionMaterializeRequest{
		LogicalSessionID: meta.ID,
		WorkspacePath:    firstNonEmpty(req.WorkspacePath, meta.WorkspacePath),
	})
	if err != nil {
		result := materializeForkFailedResult(meta, err.Error())
		return SessionMeta{}, &result, nil
	}
	if strings.TrimSpace(remote.RemoteSessionID) == "" {
		result := missingRemoteForkResult(meta, "provider returned no remote session id")
		return SessionMeta{}, &result, nil
	}
	return m.persistMaterializedForkParent(meta, remote, metadata)
}

func (m *Manager) persistMaterializedForkParent(meta SessionMeta, remote middleware.RemoteSessionInfo, metadata middleware.ConversationMetadata) (SessionMeta, *middleware.SessionActionResult, error) {
	now := time.Now().UTC()
	meta.AgentSessionID = remote.RemoteSessionID
	if remote.ProtocolKind != "" {
		meta.ProtocolKind = string(remote.ProtocolKind)
	}
	meta.MirrorStatus = "mirrored"
	meta.LastSyncedAt = now
	meta.RemoteUpdatedAt = now
	meta.RemoteTitle = firstNonEmpty(metadata.Title, remote.Title, meta.RemoteTitle)
	meta.RemoteStatus = firstNonEmpty(metadata.Status, remote.Status, meta.RemoteStatus)
	if err := m.indexSessionWorkspace(meta); err != nil {
		return SessionMeta{}, nil, err
	}
	if err := m.saveSessionMeta(meta); err != nil {
		return SessionMeta{}, nil, err
	}
	return meta, nil, nil
}

func missingRemoteForkResult(meta SessionMeta, reason string) middleware.SessionActionResult {
	result := unsupportedForkResult(meta, reason)
	result.Message = "Session cannot be forked because no remote session id is available."
	result.Error = &middleware.SessionActionError{
		Code:    "missing_remote_session_id",
		Message: "session has no remote session id to fork",
		Target:  meta.ID,
	}
	return result
}

func materializeForkFailedResult(meta SessionMeta, reason string) middleware.SessionActionResult {
	result := unsupportedForkResult(meta, reason)
	result.Message = "Session remote materialization failed before fork."
	result.Error = &middleware.SessionActionError{
		Code:    "remote_session_materialize_failed",
		Message: "failed to create remote session before fork",
		Target:  meta.ID,
	}
	return result
}
