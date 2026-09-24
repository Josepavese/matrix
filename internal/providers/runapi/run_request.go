package runapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/logic/runconfig"
	"github.com/Josepavese/matrix/internal/logic/sidecar"
)

func decodeRunRequest(w http.ResponseWriter, r *http.Request) (runRequest, bool) {
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: invalid json", http.StatusBadRequest)
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

func (s *Server) prepareRunAgentConfig(w http.ResponseWriter, req *runRequest, agentID string) bool {
	if req.ModelID != "" && s.endpointResolver != nil && s.resolveProtocol(agentID) != "acp" {
		http.Error(w, "Conflict: model_id is supported only for ACP agents", http.StatusConflict)
		return false
	}
	args, err := agentlaunch.CodexReasoningEffortArgs(agentID, req.AgentConfig.ModelReasoningEffort, req.CodexConfig.ModelReasoningEffort)
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
