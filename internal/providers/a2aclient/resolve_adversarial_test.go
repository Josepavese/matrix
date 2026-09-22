package a2aclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestResolveAgentCardRejectsUnusableSources keeps a peer from being registered
// with a card that was never fetched: an empty or malformed source must fail
// before any request is attempted.
func TestResolveAgentCardRejectsUnusableSources(t *testing.T) {
	for _, raw := range []string{"", "   ", "not a url", "://missing-scheme"} {
		if _, err := resolveAgentCard(context.Background(), raw, nil); err == nil {
			t.Fatalf("card source %q must be refused", raw)
		}
	}
}

// TestResolveAgentCardRejectsAnHTTPError keeps an error page from being parsed as
// a card: a peer that answers 500 must not enter the catalog.
func TestResolveAgentCardRejectsAnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()
	if _, err := resolveAgentCard(context.Background(), server.URL+"/card", nil); err == nil {
		t.Fatal("a failing card endpoint must be reported")
	}
}

// TestResolveAgentCardRejectsAMalformedCardDocument keeps invalid JSON from being
// accepted as an agent contract.
func TestResolveAgentCardRejectsAMalformedCardDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	}))
	defer server.Close()
	if _, err := resolveAgentCard(context.Background(), server.URL+"/card", nil); err == nil {
		t.Fatal("a malformed card must be reported")
	}
}

// TestNewClientRejectsAnUnsupportedTransport keeps a typo in an endpoint from
// producing a client that fails on the first turn.
func TestNewClientRejectsAnUnsupportedTransport(t *testing.T) {
	factory := Factory{}
	endpoint := middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindA2A, Transport: "carrier-pigeon", Address: "https://peer.example"}
	client, err := factory.NewClient(context.Background(), endpoint, middleware.ConversationFactoryDeps{})
	if err == nil {
		_ = client
		t.Fatal("an unsupported transport must be refused")
	}
	if !strings.Contains(err.Error(), "transport") && !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("the error must say what is unsupported, got %q", err)
	}
}
