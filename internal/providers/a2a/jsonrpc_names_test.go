package a2a

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestSpecJSONRPCBindingRoutesAMessageEndToEnd drives the ingress the way a
// spec-conformant client does: the JSON-RPC method name the A2A specification defines
// for the binding our agent card advertises, through the real handler and the real
// executor, to a routed answer.
//
// Before the name translation existed this returned "-32601 method not found" for
// every method a client could send, because the protocol SDK dispatches PascalCase
// identifiers while the specification's JSON-RPC binding uses slash-separated ones.
// The route was wired and the card advertised it; the messages simply never arrived.
func TestSpecJSONRPCBindingRoutesAMessageEndToEnd(t *testing.T) {
	router := &stubSessionRouter{}
	mux := http.NewServeMux()
	NewServer(router, "http://127.0.0.1:0", "opencode").RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := `{"jsonrpc":"2.0","id":"e2e-1","method":"message/send","params":{"message":{` +
		`"role":"user","kind":"message","messageId":"m-1","parts":[{"kind":"text","text":"hello from a spec client"}]}}}`
	resp, err := http.Post(server.URL+"/a2a", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	var envelope struct {
		Result struct {
			Task struct {
				Status struct {
					State string `json:"state"`
				} `json:"status"`
				Artifacts []struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"artifacts"`
			} `json:"task"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != nil {
		t.Fatalf("the spec method name was not dispatched: code=%d message=%s",
			envelope.Error.Code, envelope.Error.Message)
	}
	if state := envelope.Result.Task.Status.State; state != "TASK_STATE_COMPLETED" {
		t.Fatalf("task state = %q, want TASK_STATE_COMPLETED", state)
	}
	var answer string
	for _, artifact := range envelope.Result.Task.Artifacts {
		for _, part := range artifact.Parts {
			answer += part.Text
		}
	}
	if !strings.Contains(answer, "matrix:hello from a spec client") {
		t.Fatalf("the routed answer did not come back: %q", answer)
	}
	if router.input != "hello from a spec client" {
		t.Fatalf("the router received %q", router.input)
	}
}

// TestAnEmptyMessageEndsTheTaskInsteadOfFailingItAnonymously: a sequence that ends
// without a terminal status left a non-streaming caller with a failed task and no
// message, which the runtime logged as "processor error None". An empty message must
// say so and finish.
func TestAnEmptyMessageEndsTheTaskInsteadOfFailingItAnonymously(t *testing.T) {
	mux := http.NewServeMux()
	NewServer(&stubSessionRouter{}, "http://127.0.0.1:0", "opencode").RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	for _, method := range []string{"message/send", "SendMessage"} {
		t.Run(method, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":"empty","method":"` + method + `","params":{"message":{` +
				`"role":"user","kind":"message","messageId":"m-2","parts":[{"kind":"text","text":"   "}]}}}`
			resp, err := http.Post(server.URL+"/a2a", "application/json", strings.NewReader(body))
			if err != nil {
				t.Fatalf("post: %v", err)
			}
			defer resp.Body.Close()
			var envelope struct {
				Result struct {
					Task struct {
						Status struct {
							State string `json:"state"`
							// The SDK carries the explaining message here.
							Message *struct {
								Parts []struct {
									Text string `json:"text"`
								} `json:"parts"`
							} `json:"message"`
						} `json:"status"`
					} `json:"task"`
				} `json:"result"`
				Error *struct {
					Code int `json:"code"`
				} `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if envelope.Error != nil {
				t.Fatalf("method %q was not dispatched: code=%d", method, envelope.Error.Code)
			}
			if state := envelope.Result.Task.Status.State; state != "TASK_STATE_COMPLETED" {
				t.Fatalf("state = %q, want TASK_STATE_COMPLETED: an empty message must not fail anonymously", state)
			}
		})
	}
}

// TestTheNameTranslationLeavesEverythingElseAlone: the rewrite must not disturb a
// name the SDK already dispatches, a body that is not a JSON-RPC request, or a
// "method" key that belongs to the params.
func TestTheNameTranslationLeavesEverythingElseAlone(t *testing.T) {
	unchanged := []string{
		`{"jsonrpc":"2.0","id":"1","method":"SendMessage","params":{}}`,
		`{"jsonrpc":"2.0","id":"1","method":"GetTask","params":{"method":"message/send"}}`,
		`not json at all`,
		`{"jsonrpc":"2.0","id":"1"}`,
	}
	for _, body := range unchanged {
		if got := string(rewriteSpecMethodName([]byte(body))); got != body {
			t.Fatalf("body was rewritten:\n got %s\nwant %s", got, body)
		}
	}

	rewritten := string(rewriteSpecMethodName([]byte(
		`{"jsonrpc":"2.0","id":"1","method":"tasks/get","params":{"id":"t-1"}}`)))
	if !strings.Contains(rewritten, `"method":"GetTask"`) {
		t.Fatalf("spec method name was not translated: %s", rewritten)
	}
	if !strings.Contains(rewritten, `"id":"t-1"`) {
		t.Fatalf("translation lost the params: %s", rewritten)
	}
}

// TestTheTranslationSurvivesTheHandlerChain confirms the wrapper is transparent for a
// handler that only cares about the body it receives.
func TestTheTranslationSurvivesTheHandlerChain(t *testing.T) {
	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(payload)
		seen = string(payload)
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/a2a",
		strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"message/stream","params":{}}`))
	withSpecJSONRPCMethodNames(next).ServeHTTP(httptest.NewRecorder(), req)
	if !strings.Contains(seen, `"method":"SendStreamingMessage"`) {
		t.Fatalf("the handler saw %s", seen)
	}
	var _ = context.Background()
	var _ middleware.Content
}
