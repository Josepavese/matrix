package runapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
)

// ----------------------------------------------------------------------------
// Adversarial HTTP suite for the elicitation surface
// ----------------------------------------------------------------------------

const jsonContentType = "application/json"

func postElicitation(t *testing.T, server *Server, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, ElicitationPathV1, strings.NewReader(body))
	if mutate == nil {
		req.Header.Set("Content-Type", jsonContentType)
	} else {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	server.HandleElicitations(rec, req)
	return rec
}

func newPendingServer(t *testing.T) (*Server, *elicitation.Service) {
	t.Helper()
	service := elicitation.NewService(2 * time.Second)
	server := NewServer(nil).WithElicitationService(service)
	startPendingElicitation(t, service)
	return server, service
}

func TestElicitationRejectsUnsupportedMethods(t *testing.T) {
	server, _ := newPendingServer(t)
	for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead} {
		rec := httptest.NewRecorder()
		server.HandleElicitations(rec, httptest.NewRequest(method, ElicitationPathV1, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: expected 405, got %d", method, rec.Code)
		}
	}
}

func TestElicitationRejectsBadContentType(t *testing.T) {
	server, _ := newPendingServer(t)
	for name, contentType := range map[string]string{
		"missing":     "",
		"text":        "text/plain",
		"form":        "application/x-www-form-urlencoded",
		"json suffix": "application/json;charset=utf-8",
	} {
		rec := postElicitation(t, server, `{"elicit_id":"session:s1","action":"decline"}`, func(r *http.Request) {
			if contentType != "" {
				r.Header.Set("Content-Type", contentType)
			}
		})
		want := http.StatusUnsupportedMediaType
		if name == "json suffix" {
			// mime.ParseMediaType strips parameters, so this one is valid.
			want = http.StatusOK
		}
		if rec.Code != want {
			t.Fatalf("%s: expected %d, got %d (%s)", name, want, rec.Code, rec.Body.String())
		}
	}
}

func TestElicitationRejectsMalformedBodies(t *testing.T) {
	server, service := newPendingServer(t)
	for name, body := range map[string]string{
		"empty":         ``,
		"not json":      `{`,
		"json array":    `[]`,
		"json string":   `"decline"`,
		"json null":     `null`,
		"wrong shape":   `{"elicit_id":123,"action":true}`,
		"unknown field": `{"elicit_id":"session:s1","action":"decline","evil":"x"}`,
	} {
		rec := postElicitation(t, server, body, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d (%s)", name, rec.Code, rec.Body.String())
		}
		if len(service.Pending()) != 1 {
			t.Fatalf("%s: a rejected body must not touch the registry", name)
		}
	}
}

func TestElicitationRejectsOversizedBody(t *testing.T) {
	server, service := newPendingServer(t)
	huge := `{"elicit_id":"session:s1","action":"accept","values":{"db":"` +
		strings.Repeat("a", maxElicitationBodyBytes+1024) + `"}}`
	rec := postElicitation(t, server, huge, nil)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for an oversized body, got %d", rec.Code)
	}
	if len(service.Pending()) != 1 {
		t.Fatal("an oversized body must not resolve the request")
	}
}

func TestElicitationRejectsUnknownActions(t *testing.T) {
	server, service := newPendingServer(t)
	for _, action := range []string{"approve", "ACCEPT", "yes", "accept ", ""} {
		rec := postElicitation(t, server, `{"elicit_id":"session:s1","action":"`+action+`"}`, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("action %q: expected 400, got %d (%s)", action, rec.Code, rec.Body.String())
		}
	}
	if len(service.Pending()) != 1 {
		t.Fatal("no rejected action may resolve the request")
	}
}

func TestElicitationValidationBlocksBadValues(t *testing.T) {
	server, service := newPendingServer(t)
	cases := map[string]string{
		"unknown field":  `{"elicit_id":"session:s1","action":"accept","values":{"nope":"x"}}`,
		"wrong enum":     `{"elicit_id":"session:s1","action":"accept","values":{"db":"mysql"}}`,
		"missing field":  `{"elicit_id":"session:s1","action":"accept","values":{}}`,
		"null values":    `{"elicit_id":"session:s1","action":"accept","values":null}`,
		"wrong type":     `{"elicit_id":"session:s1","action":"accept","values":{"db":42}}`,
		"nested payload": `{"elicit_id":"session:s1","action":"accept","values":{"db":{"a":1}}}`,
	}
	for name, body := range cases {
		rec := postElicitation(t, server, body, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d (%s)", name, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "error") {
			t.Fatalf("%s: rejection must explain itself: %s", name, rec.Body.String())
		}
		if len(service.Pending()) != 1 {
			t.Fatalf("%s: invalid values must leave the request pending", name)
		}
	}
}

func TestElicitationConflictOnStaleOrDuplicateAnswer(t *testing.T) {
	server, service := newPendingServer(t)
	first := postElicitation(t, server, `{"elicit_id":"session:s1","action":"decline"}`, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first answer must succeed, got %d (%s)", first.Code, first.Body.String())
	}
	second := postElicitation(t, server, `{"elicit_id":"session:s1","action":"decline"}`, nil)
	if second.Code != http.StatusConflict {
		t.Fatalf("a second answer must conflict, got %d", second.Code)
	}
	unknown := postElicitation(t, server, `{"elicit_id":"nope","action":"decline"}`, nil)
	if unknown.Code != http.StatusConflict {
		t.Fatalf("an unknown id must conflict, got %d", unknown.Code)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(unknown.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["pending_ids"]; !ok {
		t.Fatalf("a conflict must list what is still pending: %s", unknown.Body.String())
	}
	if len(service.Pending()) != 0 {
		t.Fatal("the first answer must have cleared the registry")
	}
}

// TestElicitationConcurrentAnswersHaveOneWinner mirrors the double-tap scenario
// over HTTP: exactly one caller may be told the answer was delivered.
func TestElicitationConcurrentAnswersHaveOneWinner(t *testing.T) {
	server, service := newPendingServer(t)
	const callers = 12
	var wg sync.WaitGroup
	codes := make([]int, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			body := `{"elicit_id":"session:s1","action":"accept","values":{"db":"postgres"}}`
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, ElicitationPathV1, strings.NewReader(body))
			req.Header.Set("Content-Type", jsonContentType)
			server.HandleElicitations(rec, req)
			codes[index] = rec.Code
		}(i)
	}
	wg.Wait()
	ok, conflict := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("unexpected status %d", code)
		}
	}
	if ok != 1 || conflict != callers-1 {
		t.Fatalf("expected exactly one winner, got %d ok / %d conflict", ok, conflict)
	}
	if len(service.Pending()) != 0 {
		t.Fatal("the winning answer must clear the registry")
	}
}

// TestElicitationListShapeIsStable keeps clients from breaking on null versus
// empty arrays and on the field names the documentation promises.
func TestElicitationListShapeIsStable(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	server := NewServer(nil).WithElicitationService(service)

	rec := httptest.NewRecorder()
	server.HandleElicitations(rec, httptest.NewRequest(http.MethodGet, ElicitationPathV1, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != `{"pending":[]}` {
		t.Fatalf("empty list must serialise as an empty array, got %s", body)
	}

	startPendingElicitation(t, service)
	rec = httptest.NewRecorder()
	server.HandleElicitations(rec, httptest.NewRequest(http.MethodGet, ElicitationPathV1, nil))
	var payload struct {
		Pending []map[string]interface{} `json:"pending"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Pending) != 1 {
		t.Fatalf("expected one pending entry: %s", rec.Body.String())
	}
	entry := payload.Pending[0]
	for _, key := range []string{"id", "agent_id", "mode", "message", "fields", "session_id", "expires_at"} {
		if _, ok := entry[key]; !ok {
			t.Fatalf("pending entry is missing %q: %s", key, rec.Body.String())
		}
	}
	fields, ok := entry["fields"].([]interface{})
	if !ok || len(fields) != 1 {
		t.Fatalf("fields must be an array: %s", rec.Body.String())
	}
	field, ok := fields[0].(map[string]interface{})
	if !ok {
		t.Fatalf("field entry must be an object: %s", rec.Body.String())
	}
	if _, ok := field["options"]; !ok {
		t.Fatalf("constrained fields must expose their options: %s", rec.Body.String())
	}
}

func TestElicitationEnforcesAPIKey(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	server := NewServer(nil).WithElicitationService(service).WithAPIKey("secret")
	startPendingElicitation(t, service)

	rec := httptest.NewRecorder()
	server.HandleElicitations(rec, httptest.NewRequest(http.MethodGet, ElicitationPathV1, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a key, got %d", rec.Code)
	}

	authed := httptest.NewRequest(http.MethodGet, ElicitationPathV1, nil)
	authed.Header.Set("X-Matrix-Key", "secret")
	rec = httptest.NewRecorder()
	server.HandleElicitations(rec, authed)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with the key, got %d", rec.Code)
	}
}

// TestElicitationRoundTripsUnicodeAndLongValues keeps the neutral layer
// lossless for values that are legal but awkward.
func TestElicitationRoundTripsUnicodeAndLongValues(t *testing.T) {
	service := elicitation.NewService(time.Minute)
	server := NewServer(nil).WithElicitationService(service)
	request := middleware.ElicitationRequest{
		ID: "session:unicode", Mode: middleware.ElicitationModeForm, SessionID: "unicode",
		Fields: []middleware.ElicitationField{{Name: "nome", Type: "string", Required: true}},
	}
	asked := make(chan middleware.ElicitationOutcome, 1)
	go func() { asked <- service.Ask(context.Background(), request) }()
	deadline := time.Now().Add(time.Second)
	for len(service.Pending()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	value := "Marco 🚀 \"quoted\" \\slash\\ " + strings.Repeat("à", 200)
	body, err := json.Marshal(map[string]interface{}{
		"elicit_id": "session:unicode", "action": "accept",
		"values": map[string]interface{}{"nome": value},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := postElicitation(t, server, string(body), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	select {
	case outcome := <-asked:
		if outcome.Values["nome"] != value {
			t.Fatalf("value was altered in transit: %q", outcome.Values["nome"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("answer never reached the registry")
	}
}
