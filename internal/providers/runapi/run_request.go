package runapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/logic/runconfig"
	"github.com/Josepavese/matrix/internal/logic/sidecar"
	"github.com/Josepavese/matrix/internal/middleware"
)

func decodeRunRequest(w http.ResponseWriter, r *http.Request) (runRequest, bool) {
	// The body is decoded into memory before anything in it can be checked, so the
	// read is capped before the decoder sees it. An oversized body is answered as
	// its own case by writeRunRequestDecodeError, not as one of the json failures.
	r.Body = http.MaxBytesReader(w, r.Body, runRequestMaxBytes)
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRunRequestDecodeError(w, err)
		return runRequest{}, false
	}
	if strings.TrimSpace(req.ChannelID) == "" || strings.TrimSpace(req.Input.String()) == "" {
		http.Error(w, "Bad Request: channel_id and input are required", http.StatusBadRequest)
		return runRequest{}, false
	}
	req.SidecarCapsules = sidecar.NormalizeCapsules(req.SidecarCapsules)
	if err := sidecar.ValidateCapsules(req.SidecarCapsules); err != nil {
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return runRequest{}, false
	}
	additionalDirectories, err := runconfig.NormalizeAdditionalDirectories(req.AdditionalDirectories)
	if err != nil {
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return runRequest{}, false
	}
	req.AdditionalDirectories = additionalDirectories
	if !validateRunModels(w, &req) {
		return runRequest{}, false
	}
	if req.WorkspacePolicy != "" && req.WorkspacePolicy != "require_grant" {
		http.Error(w, "Bad Request: workspace_policy must be require_grant", http.StatusBadRequest)
		return runRequest{}, false
	}
	return req, true
}

func validateRunModels(w http.ResponseWriter, req *runRequest) bool {
	req.ModelID = strings.TrimSpace(req.ModelID)
	req.FallbackModelID = strings.TrimSpace(req.FallbackModelID)
	if len(req.ModelID) > 128 {
		http.Error(w, "Bad Request: model_id exceeds 128 bytes", http.StatusBadRequest)
		return false
	}
	if req.FallbackModelID != "" && (req.ModelID == "" || req.FallbackModelID == req.ModelID || len(req.FallbackModelID) > 128) {
		http.Error(w, "Bad Request: fallback_model_id requires a different model_id and at most 128 bytes", http.StatusBadRequest)
		return false
	}
	return true
}

// modelIDConflictMessage names the agent the request was refused for and the
// action that resolves it. The previous text said only that model_id belongs to
// ACP agents, which left the caller holding a rule instead of a next step: it
// did not say which agent had been resolved, so a caller who believed they were
// addressing an ACP agent had nothing to check.
//
// The remedies are worded to match the runtime status the operator sees for the
// same window — agentmgr reports a registered-but-unapplied agent as
// "pending_apply" with "restart the daemon or wait for the next refresh" — so
// the 409 and the status do not send the operator in two directions.
func modelIDConflictMessage(agentID string) string {
	agent := "the requested agent"
	if name := strings.TrimSpace(agentID); name != "" {
		agent = fmt.Sprintf("agent %q", name)
	}
	return "Conflict: " + agent + " is not served as an ACP agent right now, so model_id is not supported; " +
		middleware.RegistrationRemedyThen("retry")
}

func (s *Server) prepareRunAgentConfig(w http.ResponseWriter, req *runRequest, agentID string) bool {
	if req.ModelID != "" && s.endpointResolver != nil && s.resolveProtocol(agentID) != "acp" {
		http.Error(w, modelIDConflictMessage(agentID), http.StatusConflict)
		return false
	}
	declared := agentlaunch.DeclaredConfigKeysForAgent(s.endpointResolver, agentID)
	args, err := agentlaunch.ReasoningEffortArgs(declared, req.AgentConfig.ModelReasoningEffort, req.CodexConfig.ModelReasoningEffort)
	if err != nil {
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	req.agentLaunchArgs = append(req.agentLaunchArgs, args...)
	if s.endpointResolver != nil {
		if _, err := agentlaunch.ResolveForAgent(s.endpointResolver, agentID, req.agentLaunchArgs...); err != nil {
			http.Error(w, "Conflict: agent launch policy is not applicable: "+err.Error(), http.StatusConflict)
			return false
		}
	}
	return true
}
