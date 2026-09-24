package session

import (
	"errors"
	"testing"
)

func TestImportFailureKeepsDistinctRemediationCodes(t *testing.T) {
	for _, test := range []struct{ message, code string }{
		{"workspace_mismatch: provider cwd differs", "workspace_mismatch"},
		{"authentication required", "provider_auth_required"},
		{"session not found", "not_found"},
		{"ACP agent does not support session/resume or session/load", "resume_unsupported"},
		{"import requires an absolute workspace_path", "invalid_request"},
		{"transport closed", "provider_failure"},
	} {
		var failure *ImportFailure
		if !errors.As(classifyImportFailure(errors.New(test.message)), &failure) || failure.Code != test.code {
			t.Fatalf("%q classified as %+v, want %s", test.message, failure, test.code)
		}
	}
}
