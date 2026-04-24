package response

import (
	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/middleware"
)

type Success struct {
	RunID      string                           `json:"run_id"`
	Status     string                           `json:"status"`
	Output     string                           `json:"output,omitempty"`
	TraceURL   string                           `json:"trace_url"`
	EventsURL  string                           `json:"events_url"`
	ActionsURL string                           `json:"actions_url"`
	Cleanup    *middleware.SessionCleanupResult `json:"cleanup,omitempty"`
}

type Error struct {
	RunID      string                           `json:"run_id"`
	Status     string                           `json:"status"`
	Code       string                           `json:"code,omitempty"`
	Error      string                           `json:"error"`
	Details    map[string]string                `json:"details,omitempty"`
	TraceURL   string                           `json:"trace_url"`
	EventsURL  string                           `json:"events_url"`
	ActionsURL string                           `json:"actions_url"`
	Cleanup    *middleware.SessionCleanupResult `json:"cleanup,omitempty"`
}

type Builder struct {
	Prefix string
}

func (b Builder) NewSuccess(runID, status, output string, cleanup ...*middleware.SessionCleanupResult) Success {
	resp := Success{RunID: runID, Status: status, Output: output, TraceURL: b.Prefix + runID + "/trace", EventsURL: b.Prefix + runID + "/events", ActionsURL: b.Prefix + runID + "/actions"}
	if len(cleanup) > 0 {
		resp.Cleanup = cleanup[0]
	}
	return resp
}

func (b Builder) NewError(runID, status, errText string, cleanup *middleware.SessionCleanupResult) Error {
	return Error{RunID: runID, Status: status, Error: errText, TraceURL: b.Prefix + runID + "/trace", EventsURL: b.Prefix + runID + "/events", ActionsURL: b.Prefix + runID + "/actions", Cleanup: cleanup}
}

func (b Builder) NewErrorForError(runID, status string, runErr error, cleanup *middleware.SessionCleanupResult) Error {
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	resp := b.NewError(runID, status, errText, cleanup)
	if failure, ok := providerfailure.As(runErr); ok {
		resp.Code = failure.Code
		resp.Details = providerfailure.Details(failure)
	}
	return resp
}
