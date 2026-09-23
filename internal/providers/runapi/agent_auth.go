package runapi

import (
	"net/http"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// agentAuthHandler exposes the protocol's own authentication controls: which
// methods an agent publishes, and how to end its authenticated state.
//
// Why this lives on the runtime API rather than in a CLI command: the daemon
// owns the agent clients and holds the vault open, so a second process cannot
// build its own router (bbolt refuses the second writer) and would in any case
// talk to a different agent process than the one serving sessions. The runtime is
// therefore the only correct owner of these calls, and this endpoint is how an
// operator or an orchestrator reaches them.
//
// The contract:
//
//	GET  /v1/agent-auth?agent=<id>            list the advertised methods
//	POST /v1/agent-auth/logout?agent=<id>     ask the agent to log out
//
// Both are authenticated with the runtime API key when one is configured, like
// every other route on this mux.
type agentAuthHandler struct {
	controller middleware.AgentAuthenticationController
}

type agentAuthMethodsResponse struct {
	Agent   string                            `json:"agent"`
	Methods []middleware.AuthenticationMethod `json:"methods"`
}

func (s *Server) HandleAgentAuth(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r, s.apiKey) {
		return
	}
	if s.agentAuth == nil || s.agentAuth.controller == nil {
		http.Error(w, "agent authentication control not available", http.StatusServiceUnavailable)
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("agent"))
	if agentID == "" {
		http.Error(w, "missing agent query parameter", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		methods, err := s.agentAuth.controller.AgentAuthenticationMethods(r.Context(), agentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, agentAuthMethodsResponse{Agent: agentID, Methods: methods})
	case http.MethodPost:
		// Logout is the only state-changing operation here, and it is explicitly
		// addressed: there is no "authenticate" counterpart because performing a
		// login needs a human in the loop, which belongs to the onboarding flow.
		if err := s.agentAuth.controller.LogoutAgent(r.Context(), agentID); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"agent": agentID, "status": "logged_out"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
