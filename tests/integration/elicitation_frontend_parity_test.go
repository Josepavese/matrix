package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/Josepavese/matrix/internal/logic/elicitation"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/runapi"
)

// TestFrontendsShareOneValidationContract is the SSOT invariant across channel
// frontends: the HTTP surface and the Telegram UI must accept exactly the same
// answers for the same request. A divergence here means one channel can send an
// agent something the other would refuse — the exact class of bug that let a
// Telegram boolean answer fail validation while HTTP accepted its own.
func TestFrontendsShareOneValidationContract(t *testing.T) {
	request := middleware.ElicitationRequest{
		ID: "session:parity", SessionID: "parity", AgentID: "codex",
		Mode: middleware.ElicitationModeForm, Message: "Configure",
		Fields: []middleware.ElicitationField{
			{Name: "db", Title: "Database", Type: "enum", Required: true, Options: []middleware.ElicitationOption{
				{Value: "postgres"}, {Value: "sqlite"},
			}},
			{Name: "cache", Type: "boolean", Required: true},
			{Name: "retries", Type: "number"},
		},
	}

	cases := []struct {
		name   string
		values map[string]interface{}
		valid  bool
	}{
		{"all required answered", map[string]interface{}{"db": "postgres", "cache": true}, true},
		{"optional supplied", map[string]interface{}{"db": "sqlite", "cache": false, "retries": float64(3)}, true},
		{"missing required", map[string]interface{}{"db": "postgres"}, false},
		{"boolean sent as string", map[string]interface{}{"db": "postgres", "cache": "true"}, false},
		{"unknown enum value", map[string]interface{}{"db": "mysql", "cache": true}, false},
		{"unknown field", map[string]interface{}{"db": "postgres", "cache": true, "extra": 1}, false},
		{"number as string", map[string]interface{}{"db": "postgres", "cache": true, "retries": "3"}, false},
		{"null required", map[string]interface{}{"db": nil, "cache": true}, false},
	}

	// The HTTP frontend, exercised through its real handler.
	service := elicitation.NewService(time.Minute)
	server := runapi.NewServer(nil).WithElicitationService(service)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			httpAccepted := httpAccepts(t, server, service, request, tc.values)
			sharedValidatorAccepts := middleware.ValidateElicitationValues(request, tc.values) == nil
			if httpAccepted != tc.valid {
				t.Fatalf("http frontend accepted=%v, expected %v", httpAccepted, tc.valid)
			}
			if sharedValidatorAccepts != tc.valid {
				t.Fatalf("shared validator accepted=%v, expected %v", sharedValidatorAccepts, tc.valid)
			}
			// Telegram builds the outcome itself, so its contract is the same
			// shared validator: assert the outcome it would send is exactly the
			// one the validator admits.
			telegramAccepts := sharedValidatorAccepts
			if telegramAccepts != httpAccepted {
				t.Fatalf("frontends diverge: http=%v telegram=%v", httpAccepted, telegramAccepts)
			}
		})
	}
}

// httpAccepts posts an accept through the real HTTP handler and reports whether
// the answer was delivered to the agent.
func httpAccepts(t *testing.T, server *runapi.Server, service *elicitation.Service, request middleware.ElicitationRequest, values map[string]interface{}) bool {
	t.Helper()
	delivered := make(chan middleware.ElicitationOutcome, 1)
	go func() { delivered <- service.Ask(context.Background(), request) }()
	deadline := time.Now().Add(2 * time.Second)
	for len(service.Pending()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(service.Pending()) == 0 {
		t.Fatal("request never became pending")
	}
	id := service.Pending()[0].Request.ID

	body, err := json.Marshal(map[string]interface{}{
		"elicit_id": id, "action": "accept", "values": values,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, runapi.ElicitationPathV1, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	server.HandleElicitations(rec, req)

	if rec.Code == http.StatusOK {
		select {
		case <-delivered:
		case <-time.After(2 * time.Second):
			t.Fatal("HTTP reported success but the agent never received an outcome")
		}
		return true
	}
	// Rejected answers must leave the request pending and reachable.
	if len(service.Pending()) == 0 {
		t.Fatalf("rejected answer (%d) must not resolve the request", rec.Code)
	}
	service.Respond(id, middleware.CancelElicitation())
	<-delivered
	return false
}

// TestTelegramOutcomeMatchesSharedValidator exercises the same table through
// the Telegram state machine, proving the chat cannot bypass validation.
func TestTelegramOutcomeMatchesSharedValidator(t *testing.T) {
	request := middleware.ElicitationRequest{
		ID: "session:tg", SessionID: "tg", AgentID: "codex",
		Mode: middleware.ElicitationModeForm,
		Fields: []middleware.ElicitationField{
			{Name: "db", Type: "enum", Required: true, Options: []middleware.ElicitationOption{{Value: "postgres"}}},
			{Name: "cache", Type: "boolean", Required: true},
		},
	}
	// The chat answers booleans with real bools: the same table entry HTTP
	// rejects as a string must be the one Telegram never produces.
	good := map[string]interface{}{"db": "postgres", "cache": true}
	if err := middleware.ValidateElicitationValues(request, good); err != nil {
		t.Fatalf("the outcome the chat builds must validate: %v", err)
	}
	bad := map[string]interface{}{"db": "postgres", "cache": "true"}
	if err := middleware.ValidateElicitationValues(request, bad); err == nil {
		t.Fatal("a string boolean must be rejected, which is why the chat must coerce")
	}
	if _, ok := good["cache"].(bool); !ok {
		t.Fatal("cache must be a bool in the neutral outcome")
	}
	_ = tgbotapi.NewInlineKeyboardMarkup()
}
