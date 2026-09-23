package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/onboarding"
	"github.com/Josepavese/matrix/internal/logic/session"
	"github.com/Josepavese/matrix/internal/middleware"
	matrixa2a "github.com/Josepavese/matrix/internal/providers/a2a"
	"github.com/Josepavese/matrix/internal/providers/agents"
	execprov "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// ----------------------------------------------------------------------------
// A2A JSON-RPC against the real routing stack and a real ACP peer
//
// The A2A surface's own tests (internal/providers/a2a) drive the real protocol
// handler and executor over routers that answer in process, and the daemon-level
// failure recorded in issues/closed/2026-09-23-a2a-surface-not-functional.md was
// never reproduced through a real agent after the fix. This test closes that gap
// for everything below the `matrix run` entry point:
//
//   - the server is the production adapter (matrixa2a.Server, the same value
//     cmd/matrix/run.go builds), served on a real HTTP listener;
//   - the router is the production session.Manager over the production
//     agents.Router - exactly the pair the daemon wires together - not a stub;
//   - the peer is the repository's own mock ACP agent compiled from ./cmd/mock-agent
//     and spoken to over real stdio, so the answer can only have come from a real
//     process. The peer is built from its package path and is not modified.
//
// It is not a `matrix run` daemon: the HTTP server is httptest and the CLI's
// config/vault bootstrap is replaced by an in-memory store. Everything from the
// HTTP request to the agent process is the production path.
// ----------------------------------------------------------------------------

const (
	a2aE2EAgentID = "mock-agent-e2e"
	// a2aE2EAnswer is the message the mock peer sends for a default prompt. No
	// in-process stub in this repository produces it, so an artifact carrying it
	// proves a real process answered.
	a2aE2EAnswer = "I am a mock agent responding via stdio."
)

// a2aMockAgentResolver publishes the compiled mock peer as a stdio ACP endpoint for
// every agent id the test routes.
type a2aMockAgentResolver struct {
	bin string
}

func (r *a2aMockAgentResolver) GetAgentEndpoint(string) (middleware.ProtocolEndpoint, error) {
	return middleware.ProtocolEndpoint{
		Kind:      middleware.ProtocolKindACP,
		Transport: "stdio",
		Command:   r.bin,
	}, nil
}

// a2aStaticLocalizer satisfies the onboarding wizard's localizer without locale files.
type a2aStaticLocalizer struct{}

func (a2aStaticLocalizer) GetString(_, key string) string { return key }

// buildA2AMockAgent compiles ./cmd/mock-agent into a temporary directory. The
// package is owned elsewhere and is not edited: it is built from its own path.
func buildA2AMockAgent(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "mock-agent")
	build := exec.Command("go", "build", "-o", out, "./cmd/mock-agent")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the in-repo mock ACP peer: %v\n%s", err, output)
	}
	return out
}

type a2aE2EEnvelope struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type a2aE2ETask struct {
	ID        string `json:"id"`
	ContextID string `json:"contextId"`
	Status    struct {
		State   string `json:"state"`
		Message *struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"message"`
	} `json:"status"`
	Artifacts []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"artifacts"`
	History []struct {
		MessageID string `json:"messageId"`
	} `json:"history"`
}

func (task a2aE2ETask) artifactText() string {
	var text strings.Builder
	for _, artifact := range task.Artifacts {
		for _, part := range artifact.Parts {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}

// a2aE2ECall posts one JSON-RPC request to the A2A binding and returns the envelope.
func a2aE2ECall(t *testing.T, url, method, body string) a2aE2EEnvelope {
	t.Helper()
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(url+"/a2a", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /a2a %s: %v", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /a2a %s: status %d", method, resp.StatusCode)
	}
	var envelope a2aE2EEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode %s response: %v", method, err)
	}
	if envelope.Error != nil {
		t.Fatalf("%s answered error %d: %s", method, envelope.Error.Code, envelope.Error.Message)
	}
	return envelope
}

// a2aE2ETaskOf accepts both result shapes the bindings use: SendMessage wraps the task
// in {"task": ...} on the wire the SDK's JSON-RPC binding produces, while GetTask and
// CancelTask answer the task directly.
func a2aE2ETaskOf(t *testing.T, method string, result json.RawMessage) a2aE2ETask {
	t.Helper()
	var direct a2aE2ETask
	if err := json.Unmarshal(result, &direct); err == nil && direct.ID != "" {
		return direct
	}
	var wrapped struct {
		Task *a2aE2ETask `json:"task"`
	}
	if err := json.Unmarshal(result, &wrapped); err != nil || wrapped.Task == nil {
		t.Fatalf("%s: result is neither a task nor a task wrapper: %s", method, result)
	}
	return *wrapped.Task
}

// TestA2AJSONRPCCompletesATurnThroughTheRealRouterAndARealACPPeer is the missing
// end-to-end proof: a specification-named JSON-RPC request enters the production
// A2A server, is routed by the production session manager into the production agent
// router, reaches a real mock ACP peer over stdio, and comes back as a completed
// task whose artifact carries the peer's answer.
//
// The task reaching TASK_STATE_COMPLETED is itself part of the assertion: the live
// defect described in the issue failed every task while the agent was working,
// because Matrix stored a non-JSON progress payload in A2A metadata. A turn that
// completes here, with the real peer's notification flowing through the same
// notifier, exercises that fix on the real path.
func TestA2AJSONRPCCompletesATurnThroughTheRealRouterAndARealACPPeer(t *testing.T) {
	bin := buildA2AMockAgent(t)
	workspace := t.TempDir()

	store := memstore.New()
	if err := store.Set("system.configured", []byte("true")); err != nil {
		t.Fatalf("mark the in-memory home as configured: %v", err)
	}
	agentRouter := agents.NewRouter(&a2aMockAgentResolver{bin: bin})
	agentRouter.SetTrustMode(func() bool { return true })
	agentRouter.SetProcess(execprov.NewProvider())
	agentRouter.SetFS(osfs.NewFSProvider(), workspace)
	t.Cleanup(agentRouter.Close)
	manager := session.NewManager(store, agentRouter, onboarding.NewWizard(onboarding.WizardDependencies{
		Storage:   store,
		Localizer: a2aStaticLocalizer{},
	}), nil)

	a2aServer := matrixa2a.NewServer(manager, "http://127.0.0.1:0", a2aE2EAgentID)
	mux := http.NewServeMux()
	a2aServer.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	// The 1.0 name and the 0.3 name a real client sent, both through the same stack.
	for _, method := range []string{"SendMessage", "message/send"} {
		t.Run(method, func(t *testing.T) {
			messageID := "m-" + strings.NewReplacer("/", "-", "S", "s").Replace(method)
			body := fmt.Sprintf(
				`{"jsonrpc":"2.0","id":%q,"method":%q,"params":{"message":{"role":"user","messageId":%q,"metadata":{"channel_id":"a2a-e2e","agent_id":%q},"parts":[{"text":"report the workspace status"}]}}}`,
				"e2e-"+method, method, messageID, a2aE2EAgentID)

			task := a2aE2ETaskOf(t, method, a2aE2ECall(t, server.URL, method, body).Result)
			if task.Status.State != "TASK_STATE_COMPLETED" {
				t.Fatalf("state = %q (%s), want TASK_STATE_COMPLETED", task.Status.State, task.StatusMessageText())
			}
			if answer := task.artifactText(); !strings.Contains(answer, a2aE2EAnswer) {
				t.Fatalf("artifact = %q, want the peer's answer %q", answer, a2aE2EAnswer)
			}
			found := false
			for _, message := range task.History {
				if message.MessageID == messageID {
					found = true
				}
			}
			if !found {
				t.Fatalf("history = %#v, want the request message %q", task.History, messageID)
			}

			// The stored task, read back through a second operation, carries the same
			// answer - the routing reached the peer, and the task store kept it.
			fetched := a2aE2ETaskOf(t, "GetTask", a2aE2ECall(t, server.URL, "GetTask",
				fmt.Sprintf(`{"jsonrpc":"2.0","id":"get-%s","method":"GetTask","params":{"id":%q}}`, method, task.ID)).Result)
			if fetched.ID != task.ID || fetched.Status.State != "TASK_STATE_COMPLETED" {
				t.Fatalf("GetTask answered %+v, want the completed task %s", fetched, task.ID)
			}
			if answer := fetched.artifactText(); !strings.Contains(answer, a2aE2EAnswer) {
				t.Fatalf("GetTask artifact = %q, want the peer's answer %q", answer, a2aE2EAnswer)
			}
		})
	}
}

// StatusMessageText renders the failure message of a task, so a failed turn says why.
func (task a2aE2ETask) StatusMessageText() string {
	if task.Status.Message == nil {
		return "no status message"
	}
	var text strings.Builder
	for _, part := range task.Status.Message.Parts {
		text.WriteString(part.Text)
	}
	return text.String()
}
