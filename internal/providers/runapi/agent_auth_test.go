package runapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// These tests live in runapi rather than in the matrixapi wrapper because this is
// the package that owns the handler: the coverage ratchet counts the package the
// code is in, and testing it only through the wrapper left the handler's own
// branches unmeasured.

type fakeAgentAuthControl struct {
	methods []middleware.AuthenticationMethod
	err     error
	logouts []string
}

func (f *fakeAgentAuthControl) AgentAuthenticationMethods(_ context.Context, _ string) ([]middleware.AuthenticationMethod, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.methods, nil
}

func (f *fakeAgentAuthControl) AuthenticateAgent(_ context.Context, _, _ string) error { return f.err }

func (f *fakeAgentAuthControl) LogoutAgent(_ context.Context, agentID string) error {
	if f.err != nil {
		return f.err
	}
	f.logouts = append(f.logouts, agentID)
	return nil
}

func agentAuthRequest(t *testing.T, control middleware.AgentAuthenticationController, method, target string, apiKey string) *httptest.ResponseRecorder {
	t.Helper()
	server := NewServer(nil)
	if control != nil {
		server.WithAgentAuthController(control)
	}
	if apiKey != "" {
		server.WithAPIKey(apiKey)
	}
	req := httptest.NewRequest(method, target, nil)
	if apiKey != "" {
		req.Header.Set("X-Matrix-Key", apiKey)
	}
	rec := httptest.NewRecorder()
	server.HandleAgentAuth(rec, req)
	return rec
}

func TestAgentAuthHandlerListsAdvertisedMethods(t *testing.T) {
	control := &fakeAgentAuthControl{methods: []middleware.AuthenticationMethod{
		{ID: "agent-login", Type: "agent", Name: "Agent login"},
	}}
	rec := agentAuthRequest(t, control, http.MethodGet, "/v1/agent-auth?agent=codex", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var payload agentAuthMethodsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Agent != "codex" || len(payload.Methods) != 1 || payload.Methods[0].ID != "agent-login" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestAgentAuthHandlerLogoutReachesTheController(t *testing.T) {
	control := &fakeAgentAuthControl{}
	rec := agentAuthRequest(t, control, http.MethodPost, "/v1/agent-auth/logout?agent=codex", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(control.logouts) != 1 || control.logouts[0] != "codex" {
		t.Fatalf("logout did not reach the controller: %v", control.logouts)
	}
}

// TestAgentAuthHandlerRefusals covers every way the handler must say no, because a
// silent 200 on any of them would look like a successful authentication query.
func TestAgentAuthHandlerRefusals(t *testing.T) {
	cases := []struct {
		name    string
		control middleware.AgentAuthenticationController
		method  string
		target  string
		apiKey  string
		sendKey bool
		want    int
	}{
		{name: "no controller", control: nil, method: http.MethodGet, target: "/v1/agent-auth?agent=x", want: http.StatusServiceUnavailable},
		{name: "no agent", control: &fakeAgentAuthControl{}, method: http.MethodGet, target: "/v1/agent-auth", want: http.StatusBadRequest},
		{name: "blank agent", control: &fakeAgentAuthControl{}, method: http.MethodGet, target: "/v1/agent-auth?agent=%20", want: http.StatusBadRequest},
		{name: "wrong method", control: &fakeAgentAuthControl{}, method: http.MethodDelete, target: "/v1/agent-auth?agent=x", want: http.StatusMethodNotAllowed},
		{name: "unauthorized", control: &fakeAgentAuthControl{}, method: http.MethodGet, target: "/v1/agent-auth?agent=x", apiKey: "secret", want: http.StatusUnauthorized},
		{name: "agent failed", control: &fakeAgentAuthControl{err: errors.New("agent did not answer")}, method: http.MethodGet, target: "/v1/agent-auth?agent=x", want: http.StatusBadGateway},
		{name: "logout failed", control: &fakeAgentAuthControl{err: errors.New("agent did not answer")}, method: http.MethodPost, target: "/v1/agent-auth/logout?agent=x", want: http.StatusBadGateway},
		{
			name: "authorized", control: &fakeAgentAuthControl{}, method: http.MethodGet,
			target: "/v1/agent-auth?agent=x", apiKey: "secret", sendKey: true, want: http.StatusOK,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(nil)
			if tc.control != nil {
				server.WithAgentAuthController(tc.control)
			}
			apiKey := tc.apiKey
			if apiKey != "" {
				server.WithAPIKey(apiKey)
			}
			req := httptest.NewRequest(tc.method, tc.target, nil)
			if tc.sendKey {
				req.Header.Set("X-Matrix-Key", tc.apiKey)
			}
			rec := httptest.NewRecorder()
			server.HandleAgentAuth(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// TestAgentAuthControllerIsOptional: an install without a wired router must answer
// with a clear refusal rather than panicking.
func TestAgentAuthControllerIsOptional(t *testing.T) {
	if server := NewServer(nil).WithAgentAuthController(nil); server.agentAuth != nil {
		t.Fatal("a nil controller must not be stored as wired")
	}
}
