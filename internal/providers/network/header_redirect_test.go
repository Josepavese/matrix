package network

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestGovernedHeadersStayOnTheirOriginalOrigin(t *testing.T) {
	var foreignCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { foreignCalls.Add(1); fmt.Fprint(w, `{"ok":true}`) }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer origin.Close()
	p := NewProvider()
	var result map[string]any
	if err := p.FetchJSONWithHeaders(context.Background(), origin.URL, map[string]string{"X-Matrix-Key": "probe-test-key"}, &result); err == nil {
		t.Fatal("credential-bearing request followed foreign redirect")
	}
	if foreignCalls.Load() != 0 {
		t.Fatal("credential reached a different origin")
	}
	if err := p.FetchJSON(context.Background(), origin.URL, &result); err != nil {
		t.Fatalf("credential-free client policy was mutated: %v", err)
	}
	if foreignCalls.Load() != 1 || result["ok"] != true {
		t.Fatal("plain fetch did not preserve existing behavior")
	}
}
func TestGovernedHeadersSurviveSameOriginRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/runtime", http.StatusFound)
			return
		}
		if r.Header.Get("X-Matrix-Key") != "probe-test-key" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()
	var result map[string]any
	if err := NewProvider().FetchJSONWithHeaders(context.Background(), server.URL+"/start", map[string]string{"X-Matrix-Key": "probe-test-key"}, &result); err != nil {
		t.Fatal(err)
	}
	if result["ok"] != true {
		t.Fatal("same-origin response lost")
	}
}
