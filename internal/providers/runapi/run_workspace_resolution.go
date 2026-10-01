package runapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/logic/runactivity"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/session"
	"github.com/Josepavese/matrix/internal/logic/workspace"
)

// This file owns the pre-prompt half of the run contract: the workspace a run
// resolved to, and the session it expects to reuse, are decided and published
// here — before any session, agent client or prompt is touched.

// HandleRuns accepts one run, resolves its workspace identity, publishes that
// identity plus the planned session affinity, and only then dispatches.
func (s *Server) HandleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requireJSONContentType(w, r) {
		return
	}
	if !requireAPIKey(w, r, s.apiKey) {
		return
	}
	req, ok := decodeRunRequest(w, r)
	if !ok {
		return
	}
	evidence, identityErr := s.applyRunWorkspaceIdentity(&req)
	agentID := firstNonEmpty(req.AgentID, s.defaultAgent)
	if !s.prepareRunAgentConfig(w, &req, agentID) {
		return
	}
	run, ok := s.acceptNewRun(w, r, req, agentID)
	if !ok {
		return
	}
	if identityErr != nil {
		s.refuseRunWorkspace(w, run.ID, identityErr)
		return
	}
	s.publishRunWorkspacePlan(run, req, evidence)
	s.dispatchByExecutionMode(w, r, runExecution{
		runID:            run.ID,
		req:              req,
		agentID:          agentID,
		emergencyTimeout: runactivity.DurationSeconds(req.EmergencyKillSeconds),
		activityTimeout:  runactivity.DurationSeconds(req.ActivityTimeoutSeconds),
	})
}

// runWorkspaceEvidence carries both sides of the resolution: the identity the
// run will use and the hints the caller actually sent.
type runWorkspaceEvidence struct {
	identity      workspace.Identity
	requestedID   string
	requestedPath string
	comparison    workspace.Comparison
}

// applyRunWorkspaceIdentity resolves the one canonical workspace the run will
// use and writes it back into the request, so the run record and the session
// layer work on the canonical pair instead of on whichever spelling arrived. On
// refusal the request is left untouched, so the trace shows what was asked for.
//
// The comparison of the two sides is what the artifact publishes, and it is
// evaluated here so a disagreement refuses the run before the prompt instead of
// being reported after it.
func (s *Server) applyRunWorkspaceIdentity(req *runRequest) (runWorkspaceEvidence, error) {
	evidence := runWorkspaceEvidence{
		requestedID:   strings.TrimSpace(req.WorkspaceID),
		requestedPath: strings.TrimSpace(req.WorkspacePath),
	}
	identity, err := s.resolveRunWorkspaceIdentity(*req)
	evidence.identity = identity
	if err != nil {
		return evidence, err
	}
	comparison, err := workspace.CompareObservations(
		workspace.Observation{WorkspaceID: evidence.requestedID, Path: evidence.requestedPath},
		workspace.Observation{WorkspaceID: identity.ID, Path: identity.Path},
	)
	evidence.comparison = comparison
	if err != nil {
		return evidence, err
	}
	req.WorkspaceID = identity.ID
	req.WorkspacePath = identity.Path
	return evidence, nil
}

// publishRunWorkspacePlan records, before dispatch and therefore before the
// prompt, the workspace the run resolved to and the session the routing decision
// expects to reuse. Without it, adopting another workstream's remote session is
// only visible after the fact, in the post-run session event.
func (s *Server) publishRunWorkspacePlan(run runtrace.Run, req runRequest, evidence runWorkspaceEvidence) {
	identity := evidence.identity
	metadata := map[string]interface{}{
		"workspace_id":             strings.TrimSpace(identity.ID),
		"workspace_path":           strings.TrimSpace(identity.Path),
		"workspace_registered":     identity.Registered,
		"workspace_source":         identity.Source,
		"requested_workspace_id":   evidence.requestedID,
		"requested_workspace_path": evidence.requestedPath,
		"channel_id":               strings.TrimSpace(req.ChannelID),
		"agent_id":                 strings.TrimSpace(run.AgentID),
		"session_policy":           normalizeRunSessionPolicy(req.SessionPolicy),
	}
	// The requested/resolved pair travels as one block, with the fields Matrix
	// did not derive named as such: a reader of the artifact must not have to
	// reconstruct the comparison from two flat keys, nor read an absent field as
	// an agreement.
	if evidence.comparison.Requested != (workspace.Observation{}) || evidence.comparison.Resolved != (workspace.Observation{}) {
		metadata["workspace_requested"] = evidence.comparison.Requested
		metadata["workspace_resolved"] = evidence.comparison.Resolved
		metadata["workspace_not_derived"] = evidence.comparison.NotDerived
	}
	if plan, ok := s.planRunSessionAffinity(req, run.AgentID, identity); ok {
		metadata["planned_session_kind"] = plan.Kind
		metadata["planned_logical_session_id"] = plan.LogicalSessionID
		metadata["planned_remote_session_id"] = plan.RemoteSessionID
		metadata["planned_agent_id"] = plan.AgentID
		metadata["planned_new_session"] = plan.NewSession
		metadata["planned_reuses_remote_session"] = plan.ReusesRemoteSession
	}
	_, _ = s.runStore.AppendEvent(runtrace.Event{
		RunID:    run.ID,
		Kind:     "workspace.identity.resolved",
		Actor:    "matrix",
		Status:   runtrace.StatusCompleted,
		Protocol: s.resolveProtocol(run.AgentID),
		Metadata: metadata,
	})
}

// sessionAffinityPlanner is implemented by a session router that can say, before
// the prompt, which session a request will use. It is optional: a router that
// cannot answer leaves the evidence without a session prediction rather than
// having one invented for it.
type sessionAffinityPlanner interface {
	PlanSessionAffinity(channelID, agentID, workspaceID, workspacePath string) (session.SessionAffinityPlan, error)
}

// planRunSessionAffinity answers which session the run will use. An explicit
// isolated-session policy always creates a new session first, so no prediction
// is needed; otherwise the router is asked only if it can answer read-only.
func (s *Server) planRunSessionAffinity(req runRequest, agentID string, identity workspace.Identity) (session.SessionAffinityPlan, bool) {
	if runRequiresCleanup(req) {
		return session.SessionAffinityPlan{
			Kind:          "create-isolated-session",
			AgentID:       strings.TrimSpace(agentID),
			WorkspaceID:   strings.TrimSpace(identity.ID),
			WorkspacePath: strings.TrimSpace(identity.Path),
			NewSession:    true,
		}, true
	}
	planner, ok := s.router.(sessionAffinityPlanner)
	if !ok {
		return session.SessionAffinityPlan{}, false
	}
	plan, err := planner.PlanSessionAffinity(req.ChannelID, agentID, identity.ID, identity.Path)
	if err != nil {
		return session.SessionAffinityPlan{}, false
	}
	return plan, true
}

// workspaceIdentityFailure types a workspace contract error. A mismatch is a
// conflict the caller can fix; an unknown workspace is a missing reference.
func workspaceIdentityFailure(err error) error {
	if err == nil {
		return nil
	}
	failure := &providerfailure.Failure{Phase: "matrix.workspace_preflight", Diagnostics: map[string]string{"origin": "matrix"}, Err: err}
	var mismatch *workspace.MismatchError
	var disagreement *workspace.ComparisonError
	switch {
	case errors.As(err, &mismatch):
		failure.Code = WorkspaceIdentityMismatchCode
		failure.Message = "workspace_id and workspace_path denote different workspaces"
		failure.Diagnostics["workspace_id"] = mismatch.ID
		failure.Diagnostics["workspace_path"] = mismatch.RequestedPath
		failure.Diagnostics["registered_workspace_path"] = mismatch.RegisteredPath
		return failure
	case errors.As(err, &disagreement):
		failure.Code = WorkspaceIdentityMismatchCode
		failure.Message = "the requested and the resolved workspace do not agree"
		failure.Diagnostics["disagreeing_fields"] = strings.Join(disagreement.Fields, ",")
		return failure
	}
	var notFound *workspace.NotFoundError
	if errors.As(err, &notFound) {
		failure.Code = WorkspaceNotFoundCode
		failure.Message = "workspace_id is not registered in this Matrix vault"
		failure.Diagnostics["workspace_id"] = notFound.ID
		return failure
	}
	return err
}

func workspaceIdentityHTTPStatus(err error) int {
	var notFound *workspace.NotFoundError
	if errors.As(err, &notFound) {
		return http.StatusNotFound
	}
	return http.StatusConflict
}

// refuseRunWorkspace fails an already accepted run with a typed workspace
// refusal and answers the caller before any session, agent client or prompt is
// touched.
func (s *Server) refuseRunWorkspace(w http.ResponseWriter, runID string, err error) {
	err = workspaceIdentityFailure(err)
	_, _ = s.runStore.Fail(runID, err)
	providerfailure.AppendRunEvent(s.runStore, runID, err)
	writeJSON(w, workspaceIdentityHTTPStatus(err), runResponseBuilder.NewErrorForError(runID, runtrace.StatusFailed, err, nil))
}
