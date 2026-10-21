package a2a

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSpecJSONRPCBindingRoutesAMessageEndToEnd uses the current v1 binding.
func TestSpecJSONRPCBindingRoutesAMessageEndToEnd(t *testing.T) {
	router := &stubSessionRouter{}
	mux := http.NewServeMux()
	NewServer(router, "http://127.0.0.1:0", "opencode").RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := `{"jsonrpc":"2.0","id":"e2e-1","method":"SendMessage","params":{"message":{` +
		`"role":"user","kind":"message","messageId":"m-1","parts":[{"kind":"text","text":"hello from a spec client"}]}}}`
	resp, err := postCurrentA2A(server.URL+"/a2a", "application/json", strings.NewReader(body))
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

	for _, method := range []string{"SendMessage"} {
		t.Run(method, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":"empty","method":"` + method + `","params":{"message":{` +
				`"role":"user","kind":"message","messageId":"m-2","parts":[{"kind":"text","text":"   "}]}}}`
			resp, err := postCurrentA2A(server.URL+"/a2a", "application/json", strings.NewReader(body))
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
