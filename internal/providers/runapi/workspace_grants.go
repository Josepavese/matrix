package runapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/logic/workspace"
)

// Typed refusals for the workspace contract. They are Matrix-side codes: the
// provider was never asked, so they must not be reported as provider failures.
const (
	WorkspaceIdentityMismatchCode = "matrix_workspace_identity_mismatch"
	WorkspaceNotFoundCode         = "matrix_workspace_not_found"
)

type workspaceGrantRequest struct {
	RepositoryPath      string `json:"repository_path"`
	IncludeGitWorktrees bool   `json:"include_git_worktrees"`
	TTLSeconds          int    `json:"ttl_seconds"`
}

func (s *Server) HandleWorkspaceGrants(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r, s.apiKey) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listWorkspaceGrants(w, r)
	case http.MethodPost:
		if r.URL.Path != WorkspaceGrantsPathV1 {
			http.NotFound(w, r)
			return
		}
		s.registerWorkspaceGrant(w, r)
	case http.MethodDelete:
		s.revokeWorkspaceGrant(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listWorkspaceGrants(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != WorkspaceGrantsPathV1 {
		http.NotFound(w, r)
		return
	}
	grants, err := s.workspaceGrants.List()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"grants": grants})
}

func (s *Server) revokeWorkspaceGrant(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, WorkspaceGrantsPathV1+"/")
	if id == r.URL.Path || id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	if err := s.workspaceGrants.Revoke(id); err != nil {
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) registerWorkspaceGrant(w http.ResponseWriter, r *http.Request) {
	if !requireJSONContentType(w, r) {
		return
	}
	var req workspaceGrantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
		return
	}
	if req.TTLSeconds == 0 {
		req.TTLSeconds = 24 * 60 * 60
	}
	grant, err := s.workspaceGrants.Register(r.Context(), req.RepositoryPath, req.IncludeGitWorktrees, time.Duration(req.TTLSeconds)*time.Second)
	if err != nil {
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, grant)
}

// resolveRunWorkspaceIdentity resolves the one canonical workspace a run will
// use. The grant check, the session binding and the published trace metadata all
// read this answer, so a caller that names only a workspace id and a caller that
// names id and path agree on the same workspace.
func (s *Server) resolveRunWorkspaceIdentity(req runRequest) (workspace.Identity, error) {
	return s.workspaceGrants.ResolveIdentity(req.WorkspaceID, req.WorkspacePath)
}

// requireWorkspaceGrant authorizes the canonical workspace of the run. A
// workspace_id alone must work: the path the grant is evaluated on is the one
// the registry records, not the empty string a caller left out.
func (s *Server) requireWorkspaceGrant(ctx context.Context, req runRequest) error {
	identity, err := s.resolveRunWorkspaceIdentity(req)
	if err != nil {
		return workspaceIdentityFailure(err)
	}
	if req.WorkspacePolicy != "require_grant" {
		return nil
	}
	_, err = s.workspaceGrants.Authorize(ctx, identity.Path)
	if err == nil {
		return nil
	}
	return &providerfailure.Failure{
		Code: providerfailure.WorkspaceNotGranted, Phase: "matrix.workspace_preflight",
		Message: "workspace is not covered by an active Matrix Git repository grant",
		Diagnostics: map[string]string{
			"workspace_id": strings.TrimSpace(identity.ID), "workspace_path": strings.TrimSpace(identity.Path), "origin": "matrix",
		},
		Err: err,
	}
}
