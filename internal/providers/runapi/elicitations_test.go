package runapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
)

func newElicitationsTestServer(t *testing.T) (*Server, *elicitation.Service) {
	t.Helper()
	service := elicitation.NewService(2 * time.Second)
	server := NewServer(nil).WithElicitationService(service)
	return server, service
}

func startPendingElicitation(t *testing.T, service *elicitation.Service) {
	t.Helper()
	go func() {
		service.Ask(context.Background(), middleware.ElicitationRequest{
			ID:        "session:s1",
			AgentID:   "codex",
			SessionID: "s1",
			Mode:      middleware.ElicitationModeForm,
			Message:   "Which database?",
			Fields: []middleware.ElicitationField{{Name: "db", Type: "enum", Required: true, Options: []middleware.ElicitationOption{
				{Value: "postgres", Label: "Postgres"},
				{Value: "sqlite"},
			}}},
		})
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, p := range service.Pending() {
			if p.Request.ID == "session:s1" {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("elicitation never became pending")
}

func TestElicitationsGetListsPending(t *testing.T) {
	server, service := newElicitationsTestServer(t)
	startPendingElicitation(t, service)

	rec := httptest.NewRecorder()
	server.HandleElicitations(rec, httptest.NewRequest(http.MethodGet, ElicitationPathV1, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Pending []struct {
			middleware.ElicitationRequest
			ExpiresAt time.Time `json:"expires_at"`
		} `json:"pending"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Pending) != 1 || resp.Pending[0].ID != "session:s1" || len(resp.Pending[0].Fields) != 1 {
		t.Fatalf("unexpected pending list: %s", rec.Body.String())
	}
	if resp.Pending[0].AgentID != "codex" {
		t.Fatalf("pending list must name the asking agent: %s", rec.Body.String())
	}
	if resp.Pending[0].ExpiresAt.IsZero() {
		t.Fatalf("pending list must expose the interaction deadline: %s", rec.Body.String())
	}
}

func TestElicitationsPostAcceptResolvesPending(t *testing.T) {
	server, service := newElicitationsTestServer(t)
	startPendingElicitation(t, service)

	rec := httptest.NewRecorder()
	body := `{"elicit_id": "session:s1", "action": "accept", "values": {"db": "postgres"}}`
	post := httptest.NewRequest(http.MethodPost, ElicitationPathV1, strings.NewReader(body))
	post.Header.Set("Content-Type", "application/json")
	server.HandleElicitations(rec, post)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(service.Pending()) != 0 {
		t.Fatal("resolved elicitation must leave the pending list")
	}
}

func TestElicitationsPostUnknownIDIsConflict(t *testing.T) {
	server, _ := newElicitationsTestServer(t)
	rec := httptest.NewRecorder()
	body := `{"elicit_id": "nope", "action": "decline"}`
	post := httptest.NewRequest(http.MethodPost, ElicitationPathV1, strings.NewReader(body))
	post.Header.Set("Content-Type", "application/json")
	server.HandleElicitations(rec, post)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestElicitationsWithoutServiceIsUnavailable(t *testing.T) {
	server := NewServer(nil)
	rec := httptest.NewRecorder()
	server.HandleElicitations(rec, httptest.NewRequest(http.MethodGet, ElicitationPathV1, nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

// TestElicitationsRejectsInvalidValuesBeforeResolving proves a malformed
// accept never reaches the waiting agent.
func TestElicitationsRejectsInvalidValuesBeforeResolving(t *testing.T) {
	server, service := newElicitationsTestServer(t)
	startPendingElicitation(t, service)

	cases := []string{
		`{"elicit_id": "session:s1", "action": "accept", "values": {"db": "mysql"}}`,
		`{"elicit_id": "session:s1", "action": "accept", "values": {}}`,
		`{"elicit_id": "session:s1", "action": "accept", "values": {"db": "sqlite", "extra": 1}}`,
	}
	for _, body := range cases {
		rec := httptest.NewRecorder()
		post := httptest.NewRequest(http.MethodPost, ElicitationPathV1, strings.NewReader(body))
		post.Header.Set("Content-Type", "application/json")
		server.HandleElicitations(rec, post)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d: %s", body, rec.Code, rec.Body.String())
		}
		if len(service.Pending()) != 1 {
			t.Fatalf("rejected values must leave the elicitation pending: %s", body)
		}
	}
}

// TestElicitationsDeclineNeedsNoValues keeps the minimal path honest.
func TestElicitationsDeclineNeedsNoValues(t *testing.T) {
	server, service := newElicitationsTestServer(t)
	startPendingElicitation(t, service)
	rec := httptest.NewRecorder()
	post := httptest.NewRequest(http.MethodPost, ElicitationPathV1, strings.NewReader(`{"elicit_id": "session:s1", "action": "decline"}`))
	post.Header.Set("Content-Type", "application/json")
	server.HandleElicitations(rec, post)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(service.Pending()) != 0 {
		t.Fatal("decline must resolve the elicitation")
	}
}
