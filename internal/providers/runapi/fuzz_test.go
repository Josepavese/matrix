package runapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
)

// ----------------------------------------------------------------------------
// Fuzz target for the elicitation HTTP surface
// ----------------------------------------------------------------------------
//
// This endpoint is reachable without an API key on a default local install, and
// its body is parsed into a struct that decides whether a pending answer is
// resolved and what values reach the agent. The property asserted is a security
// one: an arbitrary body must never resolve a request with values that the
// shared validator would refuse.

// FuzzElicitationAnswerEndpoint asserts the endpoint's decision is consistent
// with the SSOT validator for arbitrary bodies.
func FuzzElicitationAnswerEndpoint(f *testing.F) {
	seeds := []string{
		`{"elicit_id":"session:s1","action":"accept","values":{"db":"postgres","cache":true}}`,
		`{"elicit_id":"session:s1","action":"accept","values":{"db":"mysql"}}`,
		`{"elicit_id":"session:s1","action":"accept","values":{"db":"postgres"}}`,
		`{"elicit_id":"session:s1","action":"decline"}`,
		`{"elicit_id":"session:s1","action":"cancel"}`,
		`{"elicit_id":"session:s1","action":"accept","values":{"cache":"true"}}`,
		`{"elicit_id":"session:s1","action":"accept","values":null}`,
		`{"elicit_id":"","action":"accept"}`,
		`{"elicit_id":"session:s1"}`,
		`{"action":"accept"}`,
		`{}`,
		`null`,
		`[]`,
		`{"elicit_id":123,"action":true}`,
		`{"elicit_id":"session:s1","action":"accept","values":{"db":"postgres","cache":true},"extra":1}`,
		`{"elicit_id":"session:s1","action":"ACCEPT"}`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	request := middleware.ElicitationRequest{
		ID: "session:s1", SessionID: "s1", AgentID: "codex",
		Mode: middleware.ElicitationModeForm, Message: "Configure",
		Fields: []middleware.ElicitationField{
			{Name: "db", Type: "enum", Required: true, Options: []middleware.ElicitationOption{{Value: "postgres"}, {Value: "sqlite"}}},
			{Name: "cache", Type: "boolean", Required: true},
		},
	}

	f.Fuzz(func(t *testing.T, body string) {
		service := elicitation.NewService(time.Minute)
		server := NewServer(nil).WithElicitationService(service)
		delivered := make(chan middleware.ElicitationOutcome, 1)
		go func() { delivered <- service.Ask(t.Context(), request) }()
		deadline := time.Now().Add(2 * time.Second)
		for len(service.Pending()) == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if len(service.Pending()) == 0 {
			t.Fatal("request never became pending")
		}

		req := httptest.NewRequest(http.MethodPost, ElicitationPathV1, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.HandleElicitations(rec, req)

		switch rec.Code {
		case http.StatusOK:
			// A success must have delivered an answer the validator accepts.
			select {
			case outcome := <-delivered:
				if outcome.Action == middleware.ElicitationActionAccept {
					if err := middleware.ValidateElicitationValues(request, outcome.Values); err != nil {
						t.Fatalf("endpoint delivered an answer the SSOT validator refuses: %v (body=%s)", err, body)
					}
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatalf("200 without delivering an answer (body=%s)", body)
			}
		default:
			// A rejection must leave the request pending and answerable.
			if len(service.Pending()) == 0 {
				t.Fatalf("status %d resolved the request (body=%s)", rec.Code, body)
			}
			if rec.Code >= 500 {
				t.Fatalf("status %d for a client error (body=%s)", rec.Code, body)
			}
			service.Respond(request.ID, middleware.CancelElicitation())
			<-delivered
		}
	})
}

// FuzzElicitationPendingSerialisation asserts the listing shape stays JSON-stable
// for arbitrary request content: a frontend parses it, so a marshal error here
// breaks a channel.
func FuzzElicitationPendingSerialisation(f *testing.F) {
	f.Add("message", "field", "value")
	f.Add("\x00\x01", "n", "v")
	f.Add(strings.Repeat("m", 5000), "f", "v")
	f.Fuzz(func(t *testing.T, message, fieldName, optionValue string) {
		service := elicitation.NewService(time.Minute)
		request := middleware.ElicitationRequest{
			ID: "session:fuzz", SessionID: "fuzz", Mode: middleware.ElicitationModeForm,
			Message: message, URL: "https://example.com",
			Fields: []middleware.ElicitationField{{
				Name: fieldName, Type: "enum", Required: true,
				Options: []middleware.ElicitationOption{{Value: optionValue}},
			}},
		}
		go func() { _ = service.Ask(t.Context(), request) }()
		deadline := time.Now().Add(2 * time.Second)
		for len(service.Pending()) == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		server := NewServer(nil).WithElicitationService(service)
		rec := httptest.NewRecorder()
		server.HandleElicitations(rec, httptest.NewRequest(http.MethodGet, ElicitationPathV1, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("listing failed with %d", rec.Code)
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("listing is not valid JSON: %v", err)
		}
		if _, ok := payload["pending"].([]interface{}); !ok {
			t.Fatalf("pending must always serialise as an array: %s", rec.Body.String())
		}
	})
}
