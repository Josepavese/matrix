package session

import (
	"errors"
	"strings"
)

// ImportFailure gives callers a stable reason when strict remote attachment
// fails. The provider's original error remains available through Unwrap.
type ImportFailure struct {
	Code string
	Err  error
}

func (e *ImportFailure) Error() string             { return e.Err.Error() }
func (e *ImportFailure) Unwrap() error             { return e.Err }
func (e *ImportFailure) SessionActionCode() string { return e.Code }

func classifyImportFailure(err error) error {
	if err == nil {
		return nil
	}
	var existing *ImportFailure
	if errors.As(err, &existing) {
		return err
	}
	text := strings.ToLower(err.Error())
	for _, rule := range []struct{ marker, code string }{
		{"workspace_mismatch", "workspace_mismatch"},
		{"provider_auth_required", "provider_auth_required"},
		{"authentication required", "provider_auth_required"},
		{"unauthorized", "provider_auth_required"},
		{"login required", "provider_auth_required"},
		{"not found", "not_found"},
		{"unknown session", "not_found"},
		{"does not support session/resume", "resume_unsupported"},
		{"verified remote session import is unsupported", "resume_unsupported"},
		{"requires", "invalid_request"},
		{"workspace_path", "invalid_request"},
		{"not a directory", "invalid_request"},
	} {
		if strings.Contains(text, rule.marker) {
			return &ImportFailure{Code: rule.code, Err: err}
		}
	}
	return &ImportFailure{Code: "provider_failure", Err: err}
}
