package runapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRunRequestTellsASyntaxErrorFromASchemaError pins both halves of the
// distinction the run endpoint owes its callers.
//
// A document that is not json is a syntax error. A document that is json whose
// field has the wrong shape is a schema error that names the field and the type
// the contract expects. Both used to answer "invalid json", which sent a caller
// whose document was valid to hunt for a syntax error it did not have.
func TestRunRequestTellsASyntaxErrorFromASchemaError(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantBody   []string
		rejectBody []string
	}{
		{
			name:       "broken json is a syntax error",
			body:       `{"channel_id":"c","input":"run",}`,
			wantBody:   []string{"invalid json", "syntax error"},
			rejectBody: []string{"request field"},
		},
		{
			name:       "truncated json is a syntax error",
			body:       `{"channel_id":"c","input":"run"`,
			wantBody:   []string{"invalid json", "ends before the json does"},
			rejectBody: []string{"request field"},
		},
		{
			name:       "a string where the contract declares an object names the field",
			body:       `{"channel_id":"c","input":"run","trace_policy":"default"}`,
			wantBody:   []string{`"trace_policy"`, "expected object", "got string"},
			rejectBody: []string{"invalid json"},
		},
		{
			name:       "a number where the contract declares an object names the field",
			body:       `{"channel_id":"c","input":"run","trace_policy":7}`,
			wantBody:   []string{`"trace_policy"`, "expected object", "got number"},
			rejectBody: []string{"invalid json"},
		},
		{
			name:       "a nested field is reported by its full path",
			body:       `{"channel_id":"c","input":"run","trace_policy":{"content_mode":7}}`,
			wantBody:   []string{`"trace_policy.content_mode"`, "expected string", "got number"},
			rejectBody: []string{"invalid json"},
		},
		{
			name:       "an array where the contract declares an object names the field",
			body:       `{"channel_id":"c","input":"run","trace_policy":["refs"]}`,
			wantBody:   []string{`"trace_policy"`, "expected object", "got array"},
			rejectBody: []string{"invalid json"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := &runTestRouter{}
			recorder := httptest.NewRecorder()

			NewServer(router).HandleRuns(recorder, newJSONRequest(http.MethodPost, RunPathV1, strings.NewReader(tc.body)))

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			answer := recorder.Body.String()
			for _, want := range tc.wantBody {
				if !strings.Contains(answer, want) {
					t.Fatalf("answer must name %q, got %q", want, answer)
				}
			}
			for _, reject := range tc.rejectBody {
				if strings.Contains(answer, reject) {
					t.Fatalf("answer must not claim %q, got %q", reject, answer)
				}
			}
			if router.lastConversation.ChannelID != "" {
				t.Fatalf("a rejected request must not reach the provider, routed %q", router.lastConversation.ChannelID)
			}
		})
	}
}

// TestRunRequestRejectionDoesNotEchoTheBody pins the other half of the contract:
// naming the field is the whole answer, and never the value the caller sent.
// A rejected body is exactly the kind of document that carries a credential.
func TestRunRequestRejectionDoesNotEchoTheBody(t *testing.T) {
	const secret = "sk-live-never-echo-this"
	for _, body := range []string{
		`{"channel_id":"c","input":"run","trace_policy":"` + secret + `"}`,
		`{"channel_id":"c","input":"run","trace_policy":{"content_mode":{"token":"` + secret + `"}}}`,
		`{"channel_id":"c","input":"` + secret + `"`,
	} {
		recorder := httptest.NewRecorder()
		NewServer(&runTestRouter{}).HandleRuns(recorder, newJSONRequest(http.MethodPost, RunPathV1, strings.NewReader(body)))

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want %d: %s", body, recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
		if answer := recorder.Body.String(); strings.Contains(answer, secret) {
			t.Fatalf("the rejection of %q echoed the value it was sent: %q", body, answer)
		}
	}
}
