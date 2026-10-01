package runapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/middleware"
)

// TestHandleRuns_ReasoningEffortFollowsTheProviderDeclaration drives the real
// request path for an agent whose name is not "codex".
//
// The two servers differ in one thing only: whether the endpoint publishes the
// launch contract declaring the key. Same agent name, same request body, so the
// outcome is decided by the declaration the provider published — not by who the
// agent is. A name-based gate cannot produce both results.
func TestHandleRuns_ReasoningEffortFollowsTheProviderDeclaration(t *testing.T) {
	const agentID = "mimo"

	body := func() []byte {
		encoded, _ := json.Marshal(map[string]interface{}{
			"channel_id": "halfdesk.pm",
			"agent_id":   agentID,
			"input":      "say ok",
			"agent_config": map[string]interface{}{
				"model_reasoning_effort": "xhigh",
			},
		})
		return encoded
	}

	t.Run("declared by the endpoint", func(t *testing.T) {
		router := &runTestRouter{}
		server := NewServer(router).WithTraceStorage(memstore.New()).WithEndpointResolver(launchPolicyEndpointResolver{
			endpoint: middleware.ProtocolEndpoint{
				Kind: middleware.ProtocolKindACP,
				Env:  []string{agentlaunch.ConfigKeysEnv + "=" + agentlaunch.ConfigKeyList(agentlaunch.ModelReasoningEffortKey)},
			},
		})
		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		req := newJSONRequest(http.MethodPost, RunPathV1, bytes.NewReader(body()))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if got := strings.Join(router.lastConversation.AgentLaunchArgs, "\x00"); got != "-c\x00model_reasoning_effort=\"xhigh\"" {
			t.Fatalf("unexpected agent launch args: %#v", router.lastConversation.AgentLaunchArgs)
		}
	})

	t.Run("not declared by the endpoint", func(t *testing.T) {
		router := &runTestRouter{}
		server := NewServer(router).WithTraceStorage(memstore.New()).WithEndpointResolver(launchPolicyEndpointResolver{
			endpoint: middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindACP, Env: []string{"UNRELATED=1"}},
		})
		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		req := newJSONRequest(http.MethodPost, RunPathV1, bytes.NewReader(body()))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), agentlaunch.ConfigKeysEnv) {
			t.Fatalf("the refusal should name the declaration the provider is missing, got: %s", w.Body.String())
		}
		if router.lastConversation.ChannelID != "" {
			t.Fatalf("request should not have reached router: %#v", router.lastConversation)
		}
	})
}
