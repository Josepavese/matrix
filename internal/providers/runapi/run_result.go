package runapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Josepavese/matrix/internal/logic/runactivity"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	runresponse "github.com/Josepavese/matrix/internal/providers/runapi/response"
)

// runWriteOptions is how a terminal result reaches the client. The status code
// and the streaming shape travel together because they are decided together: a
// streaming call has already sent its header and can only append.
type runWriteOptions struct {
	runID  string
	status int
	stream bool
}

// writeRunResult renders the terminal state the run actually reached. The
// request was accepted, but the verdict is the record's: reporting a fixed
// "completed" here would let a turn that produced nothing leave through the
// front door looking like work.
func writeRunResult(w http.ResponseWriter, res runExecutionResult, err error, opts runWriteOptions) {
	if isSetupRequired(err) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"code":    "SETUP_REQUIRED",
			"message": "Matrix setup is required before non-interactive /v1/runs routing.",
			"hint":    "Run `matrix bootstrap doctor` and complete setup, or set system.configured only after provisioning the required agents.",
			"run_id":  opts.runID,
		})
		return
	}
	if res.terminal != nil {
		success, failure, completed := terminalRunResult(*res.terminal, res)
		if !completed {
			writeRunPayload(w, opts, failure)
			return
		}
		writeRunPayload(w, runWriteOptions{status: http.StatusCreated, stream: opts.stream}, success)
		return
	}
	if err == nil {
		err = &runNotCompletedError{runID: opts.runID, status: runtrace.StatusUnknown}
	}
	writeRunPayload(w, opts, runResponseBuilder.NewErrorForError(opts.runID, runtrace.StatusFailed, err, res.cleanup))
}

// writeRunPayload sends one already-built response in the caller's shape.
func writeRunPayload(w http.ResponseWriter, opts runWriteOptions, payload any) {
	if opts.stream {
		_ = json.NewEncoder(w).Encode(payload)
		return
	}
	writeJSON(w, opts.status, payload)
}

// runAbort is one aborted turn: everything its terminal transition needs.
type runAbort struct {
	ctx        context.Context
	exec       runExecution
	sessionCtx runSessionContext
	res        routeResult
	activity   *runactivity.Timeout
	err        error
}

// terminalRunResult renders the terminal state the run record reached: its
// output when it completed, and otherwise the failure that explains it.
func terminalRunResult(run runtrace.Run, res runExecutionResult) (runresponse.Success, runresponse.Error, bool) {
	if run.Status == runtrace.StatusCompleted {
		return runResponseBuilder.NewSuccess(run.ID, run.Status, run.Output, res.cleanup), runresponse.Error{}, true
	}
	return runresponse.Success{}, runResponseBuilder.NewErrorForError(run.ID, run.Status, terminalFailure(run, res), res.cleanup), false
}

// terminalFailure picks what a terminal non-success response reports. A run that
// already carries a recorded error states it; a run whose record has none — a
// cancellation, above all — is explained by whatever aborted it. A typed provider
// failure is preferred in every case, so the code and diagnostics a caller
// already receives survive the terminal transition.
func terminalFailure(run runtrace.Run, res runExecutionResult) error {
	if _, typed := providerfailure.As(res.routeErr); typed {
		return res.routeErr
	}
	if run.Error != "" {
		return &runNotCompletedError{runID: run.ID, status: run.Status, detail: run.Error}
	}
	if res.abortErr != nil {
		return res.abortErr
	}
	return &runNotCompletedError{runID: run.ID, status: run.Status}
}
