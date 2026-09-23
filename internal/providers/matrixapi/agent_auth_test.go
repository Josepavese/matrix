package matrixapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// fakeAgentAuth stands in for the daemon's router: it advertises one method and
// records logouts.
type fakeAgentAuth struct {
	methods  []middleware.AuthenticationMethod
	err      error
	loggedIn []string
}

func (f *fakeAgentAuth) AgentAuthenticationMethods(_ context.Context, _ string) ([]middleware.AuthenticationMethod, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.methods, nil
}

func (f *fakeAgentAuth) AuthenticateAgent(_ context.Context, _, _ string) error {
	return f.err
}

func (f *fakeAgentAuth) LogoutAgent(_ context.Context, agentID string) error {
	if f.err != nil {
		return f.err
	}
	f.loggedIn = append(f.loggedIn, agentID)
	return nil
}

func newAgentAuthServer(t *testing.T, controller middleware.AgentAuthenticationController, apiKey string) *httptest.Server {
	t.Helper()
	server := NewServer(nil)
	server.WithAgentAuthController(controller)
	if apiKey != "" {
		server.WithAPIKey(apiKey)
	}
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// TestAgentAuthListsAdvertisedMethods is the reason this endpoint exists: while
// the daemon holds the vault, a CLI process cannot build its own router, so the
// agent's own authentication surface has to be reachable over the runtime API.
func TestAgentAuthListsAdvertisedMethods(t *testing.T) {
	controller := &fakeAgentAuth{methods: []middleware.AuthenticationMethod{{
		ID:          "opencode-login",
		Type:        "agent",
		Name:        "Login with opencode",
		Description: "Run `opencode auth login` in the terminal",
	}}}
	ts := newAgentAuthServer(t, controller, "")

	resp, err := http.Get(ts.URL + "/v1/agent-auth?agent=opencode")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var payload struct {
		Agent   string                            `json:"agent"`
		Methods []middleware.AuthenticationMethod `json:"methods"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Agent != "opencode" {
		t.Fatalf("agent = %q", payload.Agent)
	}
	if len(payload.Methods) != 1 || payload.Methods[0].ID != "opencode-login" {
		t.Fatalf("methods = %+v, want the advertised one", payload.Methods)
	}
}

// TestAgentAuthLogoutReachesTheAgent pins the write half: revoking must reach the
// controller, not just return 200.
func TestAgentAuthLogoutReachesTheAgent(t *testing.T) {
	controller := &fakeAgentAuth{}
	ts := newAgentAuthServer(t, controller, "")

	resp, err := http.Post(ts.URL+"/v1/agent-auth/logout?agent=opencode", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(controller.loggedIn) != 1 || controller.loggedIn[0] != "opencode" {
		t.Fatalf("logout did not reach the agent: %v", controller.loggedIn)
	}
}

// TestAgentAuthRequiresTheAPIKey keeps the surface governed when a key is set.
func TestAgentAuthRequiresTheAPIKey(t *testing.T) {
	controller := &fakeAgentAuth{methods: []middleware.AuthenticationMethod{{ID: "m"}}}
	ts := newAgentAuthServer(t, controller, "secret-key")

	resp, err := http.Get(ts.URL + "/v1/agent-auth?agent=opencode")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 without a key", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/agent-auth?agent=opencode", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("X-Matrix-Key", "secret-key")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET with key: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 with the key", resp2.StatusCode)
	}
}

// TestAgentAuthRefusesWithoutAnAgentAndWithoutAController covers the two failure
// shapes an operator will actually hit.
func TestAgentAuthRefusesWithoutAnAgentAndWithoutAController(t *testing.T) {
	ts := newAgentAuthServer(t, &fakeAgentAuth{}, "")
	resp, err := http.Get(ts.URL + "/v1/agent-auth")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when the agent is missing", resp.StatusCode)
	}

	bare := newAgentAuthServer(t, nil, "")
	resp2, err := http.Get(bare.URL + "/v1/agent-auth?agent=opencode")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 without a controller", resp2.StatusCode)
	}
}

// TestAgentAuthReportsAgentFailure keeps a broken agent from looking like a
// successful query.
func TestAgentAuthReportsAgentFailure(t *testing.T) {
	ts := newAgentAuthServer(t, &fakeAgentAuth{err: errors.New("agent did not answer")}, "")
	resp, err := http.Get(ts.URL + "/v1/agent-auth?agent=opencode")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
}
