package runapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
)

// elicitationsHandler serves the pending-elicitation registry: GET lists what
// agents are asking, POST answers one. This is the HTTP channel frontend for
// the neutral elicitation SSOT — the only UI surface in v1, deliberately
// programmatic: the daemon never renders forms itself, it only exposes them
// (including who is asking and how long is left).
type elicitationsHandler struct {
	service *elicitation.Service
}

type elicitationPendingDTO struct {
	middleware.ElicitationRequest
	ExpiresAt time.Time `json:"expires_at"`
}

type elicitationsResponse struct {
	Pending []elicitationPendingDTO `json:"pending"`
}

// maxElicitationBodyBytes bounds an answer body. Values are form answers, not
// documents, and the endpoint is reachable without an API key on a default
// local install, so an unbounded decode would be a memory amplifier.
const maxElicitationBodyBytes = 64 * 1024

type elicitationRespondRequest struct {
	// ElicitID matches the pending request ID from GET.
	ElicitID string                 `json:"elicit_id"`
	Action   string                 `json:"action"` // accept | decline | cancel
	Values   map[string]interface{} `json:"values,omitempty"`
}

func (s *Server) HandleElicitations(w http.ResponseWriter, r *http.Request) {
	if !requireAPIKey(w, r, s.apiKey) {
		return
	}
	if s.elicitations == nil || s.elicitations.service == nil {
		http.Error(w, "elicitation frontend not enabled", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, elicitationsResponse{Pending: pendingDTOs(s.elicitations.service)})
	case http.MethodPost:
		s.handleElicitationRespond(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleElicitationRespond(w http.ResponseWriter, r *http.Request) {
	if !requireJSONContentType(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxElicitationBodyBytes)
	var req elicitationRespondRequest
	decoder := json.NewDecoder(r.Body)
	// Answer bodies are small and machine-written: rejecting unknown fields
	// turns a silent typo (value vs values) into a diagnosable error instead of
	// an answer that quietly carries nothing.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]interface{}{
				"error": "answer body exceeds the accepted size",
				"limit": maxElicitationBodyBytes,
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid JSON body"})
		return
	}
	req.ElicitID = strings.TrimSpace(req.ElicitID)
	if req.ElicitID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "elicit_id is required"})
		return
	}
	outcome, ok := outcomeFor(req)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "unknown action: " + req.Action,
			"allowed": []string{middleware.ElicitationActionAccept, middleware.ElicitationActionDecline, middleware.ElicitationActionCancel},
		})
		return
	}
	// Validate accepted values against the schema the user was shown before
	// resolving anything, so a malformed answer never reaches the agent.
	if outcome.Action == middleware.ElicitationActionAccept {
		if err := validateAgainst(s.elicitations.service, req.ElicitID, outcome.Values); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"error":     err.Error(),
				"elicit_id": req.ElicitID,
			})
			return
		}
	}
	if !s.elicitations.service.Respond(req.ElicitID, outcome) {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error":       "elicit_id is not pending",
			"elicit_id":   req.ElicitID,
			"pending_ids": pendingIDs(s.elicitations.service),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"elicit_id": req.ElicitID, "action": outcome.Action})
}

func outcomeFor(req elicitationRespondRequest) (middleware.ElicitationOutcome, bool) {
	switch req.Action {
	case middleware.ElicitationActionAccept:
		return middleware.AcceptElicitation(req.Values), true
	case middleware.ElicitationActionDecline:
		return middleware.DeclineElicitation(), true
	case middleware.ElicitationActionCancel:
		return middleware.CancelElicitation(), true
	default:
		return middleware.ElicitationOutcome{}, false
	}
}

// validateAgainst looks the pending request up so validation runs against the
// schema actually shown to the user, not against caller-supplied input.
func validateAgainst(service *elicitation.Service, id string, values map[string]interface{}) error {
	for _, p := range service.Pending() {
		if p.Request.ID == id {
			return middleware.ValidateElicitationValues(p.Request, values)
		}
	}
	return nil // unknown IDs are reported as a conflict by Respond
}

func pendingDTOs(service *elicitation.Service) []elicitationPendingDTO {
	pending := service.Pending()
	out := make([]elicitationPendingDTO, 0, len(pending))
	for _, p := range pending {
		out = append(out, elicitationPendingDTO{ElicitationRequest: p.Request, ExpiresAt: p.ExpiresAt})
	}
	return out
}

func pendingIDs(service *elicitation.Service) []string {
	pending := service.Pending()
	ids := make([]string, 0, len(pending))
	for _, p := range pending {
		ids = append(ids, p.Request.ID)
	}
	return ids
}
