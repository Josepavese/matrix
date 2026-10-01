package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/providers/runapi"
)

// submissionSeen is what the fake runtime recorded about the request it received.
type submissionSeen struct {
	method  string
	path    string
	ctype   string
	key     string
	idemKey string
	body    map[string]any
}

// serveSubmission starts a runtime that answers one submission, and reports what
// the client actually sent.
func serveSubmission(t *testing.T, status int, headers map[string]string, payload string) (*httptest.Server, *submissionSeen) {
	t.Helper()
	seen := &submissionSeen{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		seen.method, seen.path, seen.ctype = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		seen.key, seen.idemKey = r.Header.Get("X-Matrix-Key"), r.Header.Get(ackIdempotencyKeyHeader)
		_ = json.Unmarshal(raw, &seen.body)
		for name, value := range headers {
			w.Header().Set(name, value)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(server.Close)
	return server, seen
}

func submissionAddress(t *testing.T, server *httptest.Server) string {
	t.Helper()
	return strings.TrimPrefix(server.URL, "http://")
}

// TestSubmitRunPostsTheExistingRunContract is the acceptance evidence for the
// first half of the delivery primitive: a submission goes to the route the
// Matrix HTTP API already publishes, with the credential and the idempotency key
// the caller chose, and the run_id it answered is what the caller waits on.
func TestSubmitRunPostsTheExistingRunContract(t *testing.T) {
	server, seen := serveSubmission(t, http.StatusCreated, nil,
		`{"run_id":"run-submitted","status":"running","trace_url":"/v1/runs/run-submitted/trace"}`)

	result, err := submitRun(context.Background(), runSubmitInput{
		Address:        submissionAddress(t, server),
		APIKey:         "matrix-key",
		AgentID:        "mimo",
		Prompt:         "quanto fa 2+2",
		IdempotencyKey: "submit-1",
		Timeout:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("submitRun: %v", err)
	}
	if result.RunID != "run-submitted" || result.Status != "running" {
		t.Fatalf("result = %+v", result)
	}
	if result.Replayed {
		t.Fatal("a fresh submission must not be reported as a replay")
	}
	if seen.method != http.MethodPost || seen.path != runapi.RunPathV1 {
		t.Fatalf("submitted %s %s, want POST %s", seen.method, seen.path, runapi.RunPathV1)
	}
	if seen.ctype != "application/json" {
		t.Fatalf("content type = %q", seen.ctype)
	}
	if seen.key != "matrix-key" {
		t.Fatalf("the surface credential was not sent: X-Matrix-Key = %q", seen.key)
	}
	if seen.idemKey != "submit-1" {
		t.Fatalf("the idempotency key was not sent: %q", seen.idemKey)
	}
	if seen.body["input"] != "quanto fa 2+2" || seen.body["agent_id"] != "mimo" {
		t.Fatalf("body = %+v", seen.body)
	}
}

// TestSubmitRunReportsAReplayAsAReplay keeps a repeated submission from reading
// as a second run: the runtime answers the run it already created, and the
// command has to say so.
func TestSubmitRunReportsAReplayAsAReplay(t *testing.T) {
	server, _ := serveSubmission(t, http.StatusCreated,
		map[string]string{ackReplayedHeader: "true"},
		`{"run_id":"run-submitted","status":"running"}`)

	result, err := submitRun(context.Background(), runSubmitInput{
		Address: submissionAddress(t, server), Prompt: "x", IdempotencyKey: "submit-1",
	})
	if err != nil {
		t.Fatalf("submitRun: %v", err)
	}
	if !result.Replayed {
		t.Fatal("a replayed submission must be reported as a replay")
	}
}

// TestSubmitRunRefusesRatherThanClaimingAnAcceptedRun pins the two refusals a
// caller must be able to tell apart from an accepted run: the surface rejecting
// the request, and an answer with no run_id to wait on.
func TestSubmitRunRefusesRatherThanClaimingAnAcceptedRun(t *testing.T) {
	rejected, _ := serveSubmission(t, http.StatusUnauthorized, nil, "Unauthorized")
	if _, err := submitRun(context.Background(), runSubmitInput{
		Address: submissionAddress(t, rejected), Prompt: "x",
	}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("a refused submission must be reported as refused, got err = %v", err)
	}

	empty, _ := serveSubmission(t, http.StatusCreated, nil, `{"status":"running"}`)
	if _, err := submitRun(context.Background(), runSubmitInput{
		Address: submissionAddress(t, empty), Prompt: "x",
	}); err == nil || !strings.Contains(err.Error(), "no run_id") {
		t.Fatalf("a run with no identity must not be reported as submitted, got err = %v", err)
	}
}

// TestSubmitRunUsesTheSurfaceKeyTheRuntimeEnforces is the client half of the
// 401 defect: the Matrix HTTP surface authenticates with its own configured key,
// and a client that sends nothing (or the other surface's key) is refused by the
// runtime's real handler, not by a stub that agrees with the client.
func TestSubmitRunUsesTheSurfaceKeyTheRuntimeEnforces(t *testing.T) {
	runtime := runapi.NewServer(nil).WithAPIKey("matrix-api-key")
	server := httptest.NewServer(http.HandlerFunc(runtime.HandleRuns))
	t.Cleanup(server.Close)
	address := submissionAddress(t, server)

	if _, err := submitRun(context.Background(), runSubmitInput{
		Address: address, APIKey: "daemon-api-key", Prompt: "x", Timeout: 5 * time.Second,
	}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("the other surface's key must be refused, got err = %v", err)
	}
}

// TestMatrixHTTPBaseURLDialsLoopbackForAWildcardBind keeps the client from
// dialing 0.0.0.0, which is a bind instruction rather than an address, while
// leaving a specific bind untouched.
func TestMatrixHTTPBaseURLDialsLoopbackForAWildcardBind(t *testing.T) {
	for _, test := range []struct{ address, want string }{
		{address: "0.0.0.0:9091", want: "http://127.0.0.1:9091"},
		{address: "127.0.0.1:9091", want: "http://127.0.0.1:9091"},
		{address: "[::]:9091", want: "http://[::1]:9091"},
		{address: "[::1]:9091", want: "http://[::1]:9091"},
	} {
		got, err := matrixHTTPBaseURL(test.address)
		if err != nil || got != test.want {
			t.Fatalf("matrixHTTPBaseURL(%q) = %q, %v; want %q", test.address, got, err, test.want)
		}
	}
	if _, err := matrixHTTPBaseURL("9091"); err == nil {
		t.Fatal("an address that is not host:port must be refused, not guessed")
	}
}

// TestSubmitPromptRefusesWhatItCannotSend keeps an empty prompt from becoming a
// successful run with no output, which reads as a broken agent.
func TestSubmitPromptRefusesWhatItCannotSend(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(empty, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ why, prompt, file string }{
		{why: "neither source", prompt: "", file: ""},
		{why: "both sources", prompt: "hello", file: empty},
		{why: "empty file", prompt: "", file: empty},
	} {
		if _, err := submitPromptText(test.prompt, test.file); err == nil {
			t.Fatalf("%s must be refused", test.why)
		}
	}
	got, err := submitPromptText("saluta", "")
	if err != nil || got != "saluta" {
		t.Fatalf("submitPromptText = %q, %v", got, err)
	}
}
