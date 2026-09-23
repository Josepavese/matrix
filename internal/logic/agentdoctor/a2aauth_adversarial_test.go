package agentdoctor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/a2aclient"
)

// remoteCardWithAPIKey builds the smallest declared card that demands one API
// key header.
func remoteCardWithAPIKey(header string) *a2aclient.RemoteAuthCard {
	return &a2aclient.RemoteAuthCard{
		Schemes: []a2aclient.RemoteAuthScheme{{
			Name: "peerKey", Kind: a2aclient.RemoteAuthAPIKey, In: a2aclient.RemoteAuthInHeader, Parameter: header,
		}},
		Requirements: []a2aclient.RemoteAuthRequirement{{Names: []string{"peerKey"}}},
	}
}

// remoteCardWithBearer builds the smallest declared card that demands an HTTP
// bearer token.
func remoteCardWithBearer() *a2aclient.RemoteAuthCard {
	return &a2aclient.RemoteAuthCard{
		Schemes:      []a2aclient.RemoteAuthScheme{{Name: "peerBearer", Kind: a2aclient.RemoteAuthHTTP, Scheme: "Bearer"}},
		Requirements: []a2aclient.RemoteAuthRequirement{{Names: []string{"peerBearer"}}},
	}
}

// TestEvaluateA2AAuthenticationJudgesAPIKeyHeader pins the apiKey rule: the
// header the card names must be configured for the endpoint, and an absent one
// is a warning that names the missing header.
func TestEvaluateA2AAuthenticationJudgesAPIKeyHeader(t *testing.T) {
	card := remoteCardWithAPIKey("X-Peer-Key")

	present, warnings := EvaluateA2AAuthentication(card, map[string]string{"x-peer-key": "secret"})
	if present.Status != A2AAuthSatisfied {
		t.Fatalf("a configured apiKey header must read as satisfied, got %q (%s)", present.Status, present.Detail)
	}
	if len(warnings) != 0 {
		t.Fatalf("a satisfied requirement must not warn, got %v", warnings)
	}

	absent, warnings := EvaluateA2AAuthentication(card, map[string]string{"Authorization": "Bearer unrelated"})
	if absent.Status != A2AAuthMissing {
		t.Fatalf("a missing apiKey header must read as missing, got %q", absent.Status)
	}
	if len(warnings) == 0 {
		t.Fatal("a missing apiKey header must warn")
	}
	if !strings.Contains(warnings[0], "X-Peer-Key") {
		t.Fatalf("the warning must name the missing header, got %q", warnings[0])
	}

	blank, warnings := EvaluateA2AAuthentication(card, map[string]string{"X-Peer-Key": "   "})
	if blank.Status != A2AAuthMissing || len(warnings) == 0 {
		t.Fatalf("a blank apiKey header is not a credential, got %q warnings=%v", blank.Status, warnings)
	}
}

// TestEvaluateA2AAuthenticationJudgesBearerAuthorization pins the http bearer
// rule: only an Authorization header whose value starts with "Bearer " counts,
// and a non-bearer Authorization value is a warning, not a pass.
func TestEvaluateA2AAuthenticationJudgesBearerAuthorization(t *testing.T) {
	card := remoteCardWithBearer()

	bearer, warnings := EvaluateA2AAuthentication(card, map[string]string{"authorization": "Bearer token-1"})
	if bearer.Status != A2AAuthSatisfied || len(warnings) != 0 {
		t.Fatalf("a Bearer Authorization header must read as satisfied, got %q warnings=%v", bearer.Status, warnings)
	}

	for name, value := range map[string]string{
		"basic value":     "Basic dXNlcjpwYXNz",
		"empty bearer":    "Bearer ",
		"bearerless word": "Bearerless",
	} {
		t.Run(name, func(t *testing.T) {
			report, warnings := EvaluateA2AAuthentication(card, map[string]string{"Authorization": value})
			if report.Status != A2AAuthMissing {
				t.Fatalf("Authorization %q must not satisfy a bearer scheme, got %q", value, report.Status)
			}
			if len(warnings) == 0 || !strings.Contains(warnings[0], "Bearer") {
				t.Fatalf("the warning must say what a bearer scheme needs, got %v", warnings)
			}
		})
	}

	absent, warnings := EvaluateA2AAuthentication(card, nil)
	if absent.Status != A2AAuthMissing || len(warnings) == 0 {
		t.Fatalf("no configured headers must read as missing, got %q warnings=%v", absent.Status, warnings)
	}
}

// TestEvaluateA2AAuthenticationReportsUnverifiableSchemes keeps a scheme that
// is not carried by headers from being reported as satisfied or as missing:
// doctor says it cannot tell, and does not raise a false alarm.
func TestEvaluateA2AAuthenticationReportsUnverifiableSchemes(t *testing.T) {
	cases := map[string]a2aclient.RemoteAuthScheme{
		"oauth2":        {Name: "peerScheme", Kind: a2aclient.RemoteAuthOAuth2},
		"openIdConnect": {Name: "peerScheme", Kind: a2aclient.RemoteAuthOpenIDConnect},
		"mutualTLS":     {Name: "peerScheme", Kind: a2aclient.RemoteAuthMutualTLS},
		"unknown":       {Name: "peerScheme", Kind: a2aclient.RemoteAuthUnknown},
		"http basic":    {Name: "peerScheme", Kind: a2aclient.RemoteAuthHTTP, Scheme: "Basic"},
	}
	for name, scheme := range cases {
		t.Run(name, func(t *testing.T) {
			card := &a2aclient.RemoteAuthCard{
				Schemes:      []a2aclient.RemoteAuthScheme{scheme},
				Requirements: []a2aclient.RemoteAuthRequirement{{Names: []string{"peerScheme"}}},
			}
			report, warnings := EvaluateA2AAuthentication(card, map[string]string{"X-Peer-Key": "secret"})
			if report.Status != A2AAuthUnverifiable {
				t.Fatalf("%s cannot be verified from headers, got %q", name, report.Status)
			}
			if len(warnings) != 0 {
				t.Fatalf("%s is unverifiable, not missing: it must not warn, got %v", name, warnings)
			}
			if len(report.Requirements) != 1 || len(report.Requirements[0].Schemes) != 1 {
				t.Fatalf("the report must describe the declared scheme, got %+v", report.Requirements)
			}
			if detail := report.Requirements[0].Schemes[0].Detail; !strings.Contains(detail, "headers") {
				t.Fatalf("the detail must say why it cannot be verified, got %q", detail)
			}
		})
	}

	// An apiKey carried in the query string is outside what headers can prove.
	queryKey := &a2aclient.RemoteAuthCard{
		Schemes: []a2aclient.RemoteAuthScheme{{
			Name: "peerKey", Kind: a2aclient.RemoteAuthAPIKey, In: a2aclient.RemoteAuthInQuery, Parameter: "key",
		}},
		Requirements: []a2aclient.RemoteAuthRequirement{{Names: []string{"peerKey"}}},
	}
	report, warnings := EvaluateA2AAuthentication(queryKey, map[string]string{"key": "secret"})
	if report.Status != A2AAuthUnverifiable || len(warnings) != 0 {
		t.Fatalf("a query apiKey cannot be verified from headers, got %q warnings=%v", report.Status, warnings)
	}
}

// TestEvaluateA2AAuthenticationHonorsAlternativeRequirements pins the OpenAPI
// OR-of-ANDs shape: a card offering a key or a bearer token is satisfied when
// either is configured, so a missing alternative must not warn.
func TestEvaluateA2AAuthenticationHonorsAlternativeRequirements(t *testing.T) {
	card := &a2aclient.RemoteAuthCard{
		Schemes: []a2aclient.RemoteAuthScheme{
			{Name: "peerBearer", Kind: a2aclient.RemoteAuthHTTP, Scheme: "Bearer"},
			{Name: "peerKey", Kind: a2aclient.RemoteAuthAPIKey, In: a2aclient.RemoteAuthInHeader, Parameter: "X-Peer-Key"},
		},
		Requirements: []a2aclient.RemoteAuthRequirement{
			{Names: []string{"peerKey"}},
			{Names: []string{"peerBearer"}},
		},
	}

	report, warnings := EvaluateA2AAuthentication(card, map[string]string{"Authorization": "Bearer token-1"})
	if report.Status != A2AAuthSatisfied {
		t.Fatalf("one satisfied alternative must satisfy the card, got %q", report.Status)
	}
	if len(warnings) != 0 {
		t.Fatalf("a satisfied alternative must not warn, got %v", warnings)
	}

	report, warnings = EvaluateA2AAuthentication(card, nil)
	if report.Status != A2AAuthMissing || len(warnings) == 0 {
		t.Fatalf("no alternative satisfiable must read as missing, got %q warnings=%v", report.Status, warnings)
	}
	if !strings.Contains(warnings[0], "X-Peer-Key") || !strings.Contains(warnings[0], "Bearer") {
		t.Fatalf("the warning must name both unsatisfied credentials, got %q", warnings[0])
	}

	anonymous, warnings := EvaluateA2AAuthentication(&a2aclient.RemoteAuthCard{
		Schemes:      card.Schemes,
		Requirements: []a2aclient.RemoteAuthRequirement{{}},
	}, nil)
	if anonymous.Status != A2AAuthSatisfied || len(warnings) != 0 {
		t.Fatalf("an empty requirement means anonymous access is allowed, got %q warnings=%v", anonymous.Status, warnings)
	}
}

// TestEvaluateA2AAuthenticationRejectsAnEmptyContract keeps a card that names a
// scheme it never declares from reading as satisfied.
func TestEvaluateA2AAuthenticationRejectsAnEmptyContract(t *testing.T) {
	report, warnings := EvaluateA2AAuthentication(&a2aclient.RemoteAuthCard{
		Requirements: []a2aclient.RemoteAuthRequirement{{Names: []string{"ghost"}}},
	}, map[string]string{"X-Peer-Key": "secret"})
	if report.Status != A2AAuthUnverifiable || len(warnings) != 0 {
		t.Fatalf("an undeclared scheme cannot be judged, got %q warnings=%v", report.Status, warnings)
	}
	if len(report.Requirements[0].Schemes) != 1 || !strings.Contains(report.Requirements[0].Schemes[0].Detail, "does not define") {
		t.Fatalf("the report must say the scheme is undeclared, got %+v", report.Requirements)
	}

	notDeclared, warnings := EvaluateA2AAuthentication(&a2aclient.RemoteAuthCard{}, map[string]string{"X-Peer-Key": "secret"})
	if notDeclared.Status != A2AAuthNotDeclared || len(warnings) != 0 {
		t.Fatalf("a card without security requirements needs no credential, got %q warnings=%v", notDeclared.Status, warnings)
	}

	nilCard, warnings := EvaluateA2AAuthentication(nil, nil)
	if nilCard == nil || nilCard.Status != A2AAuthCardUnavailable || len(warnings) != 0 {
		t.Fatalf("a nil card must be reported, got %+v warnings=%v", nilCard, warnings)
	}
}

// TestInspectA2AAuthenticationWarnsWhenTheCardIsUnavailable keeps the extra
// network read from failing the doctor run: an unreadable card is a warning and
// a report that says so, never an error.
func TestInspectA2AAuthenticationWarnsWhenTheCardIsUnavailable(t *testing.T) {
	endpoint := middleware.ProtocolEndpoint{
		Kind:    middleware.ProtocolKindA2A,
		Address: "https://peer.example",
		Headers: map[string]string{"X-Peer-Key": "secret"},
	}

	report, warnings := InspectA2AAuthentication(context.Background(), endpoint, func(context.Context, string, map[string]string) (*a2aclient.RemoteAuthCard, error) {
		return nil, errors.New("connection refused")
	})
	if report == nil || report.Status != A2AAuthCardUnavailable {
		t.Fatalf("an unreadable card must be reported as unavailable, got %+v", report)
	}
	if report.CardURL != endpoint.Address {
		t.Fatalf("the report must name the card source it used, got %q", report.CardURL)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "connection refused") {
		t.Fatalf("the failure must warn with its cause, got %v", warnings)
	}

	nilCard, warnings := InspectA2AAuthentication(context.Background(), endpoint, func(context.Context, string, map[string]string) (*a2aclient.RemoteAuthCard, error) {
		return nil, nil
	})
	if nilCard == nil || nilCard.Status != A2AAuthCardUnavailable || len(warnings) != 1 {
		t.Fatalf("a resolver that returns nothing must warn, got %+v warnings=%v", nilCard, warnings)
	}
}

// TestInspectA2AAuthenticationSkipsWhatItDoesNotOwn keeps the preflight from
// probing anything that is not a targeted A2A endpoint.
func TestInspectA2AAuthenticationSkipsWhatItDoesNotOwn(t *testing.T) {
	refuse := func(context.Context, string, map[string]string) (*a2aclient.RemoteAuthCard, error) {
		t.Fatal("doctor must not read a card it has no reason to read")
		return nil, nil
	}
	cases := map[string]middleware.ProtocolEndpoint{
		"ACP endpoint":            {Kind: middleware.ProtocolKindACP, Transport: "stdio", Command: "/usr/bin/codex"},
		"A2A without target":      {Kind: middleware.ProtocolKindA2A, Transport: "JSONRPC"},
		"A2A without card or URL": {Kind: middleware.ProtocolKindA2A, Transport: "JSONRPC", Tenant: "tenant-only"},
	}
	for name, endpoint := range cases {
		t.Run(name, func(t *testing.T) {
			report, warnings := InspectA2AAuthentication(context.Background(), endpoint, refuse)
			if report != nil || len(warnings) != 0 {
				t.Fatalf("expected no report, got report=%+v warnings=%v", report, warnings)
			}
		})
	}

	if report, warnings := InspectA2AAuthentication(context.Background(), middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindA2A, Address: "https://peer.example"}, nil); report != nil || len(warnings) != 0 {
		t.Fatalf("a nil fetcher must produce no report, got %+v warnings=%v", report, warnings)
	}
}

// TestInspectA2AAuthenticationReadsTheConfiguredCardSource pins the plumbing:
// the injected fetcher receives the card URL when one is configured, otherwise
// the endpoint address, and always receives the configured headers.
func TestInspectA2AAuthenticationReadsTheConfiguredCardSource(t *testing.T) {
	headers := map[string]string{"X-Peer-Key": "secret"}
	cases := map[string]struct {
		endpoint middleware.ProtocolEndpoint
		wantURL  string
	}{
		"card URL wins": {
			endpoint: middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindA2A, CardURL: "https://peer.example/card.json", Address: "https://peer.example"},
			wantURL:  "https://peer.example/card.json",
		},
		"address is the base URL": {
			endpoint: middleware.ProtocolEndpoint{Kind: middleware.ProtocolKindA2A, Address: "https://peer.example"},
			wantURL:  "https://peer.example",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tc.endpoint.Headers = headers
			var gotURL string
			var gotHeaders map[string]string
			report, warnings := InspectA2AAuthentication(context.Background(), tc.endpoint, func(_ context.Context, cardURL string, requestHeaders map[string]string) (*a2aclient.RemoteAuthCard, error) {
				gotURL, gotHeaders = cardURL, requestHeaders
				return remoteCardWithAPIKey("X-Peer-Key"), nil
			})
			if gotURL != tc.wantURL {
				t.Fatalf("the fetcher must read %q, got %q", tc.wantURL, gotURL)
			}
			if gotHeaders["X-Peer-Key"] != "secret" {
				t.Fatalf("the fetcher must receive the endpoint headers, got %v", gotHeaders)
			}
			if report == nil || report.CardURL != tc.wantURL || report.Status != A2AAuthSatisfied || len(warnings) != 0 {
				t.Fatalf("unexpected report %+v warnings=%v", report, warnings)
			}
		})
	}
}

// TestInspectA2AAuthenticationUsesTheOutboundCardResolver proves the preflight
// reads through the same resolver the client uses, including the well-known card
// path for an address-only endpoint, the configured request headers, and the
// card's own JSON security shapes.
func TestInspectA2AAuthenticationUsesTheOutboundCardResolver(t *testing.T) {
	var sawHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent-card.json" {
			http.NotFound(w, r)
			return
		}
		sawHeader = r.Header.Get("X-Peer-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "peer",
			"version": "1",
			"securitySchemes": {
				"peerKey": {"apiKeySecurityScheme": {"name": "X-Peer-Key", "location": "header"}},
				"peerSSO": {"openIdConnectSecurityScheme": {"openIdConnectUrl": "https://peer.example/.well-known/openid-configuration"}}
			},
			"securityRequirements": [{"schemes": {"peerKey": []}}, {"schemes": {"peerSSO": []}}]
		}`))
	}))
	defer server.Close()

	endpoint := middleware.ProtocolEndpoint{
		Kind:    middleware.ProtocolKindA2A,
		Address: server.URL,
		Headers: map[string]string{"X-Peer-Key": "secret"},
	}
	report, warnings := InspectA2AAuthentication(context.Background(), endpoint, a2aclient.FetchRemoteAuthCard)
	if sawHeader != "secret" {
		t.Fatalf("the card request must carry the configured header, got %q", sawHeader)
	}
	if report == nil || report.Status != A2AAuthSatisfied || len(warnings) != 0 {
		t.Fatalf("the served card must read as satisfied, got %+v warnings=%v", report, warnings)
	}
	if report.CardURL != server.URL || len(report.DeclaredSchemes) != 2 {
		t.Fatalf("unexpected report %+v", report)
	}

	delete(endpoint.Headers, "X-Peer-Key")
	report, warnings = InspectA2AAuthentication(context.Background(), endpoint, a2aclient.FetchRemoteAuthCard)
	if report == nil || report.Status != A2AAuthUnverifiable || len(warnings) != 0 {
		t.Fatalf("without the key only the unverifiable alternative remains, got %+v warnings=%v", report, warnings)
	}

	unreachable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "card unavailable", http.StatusInternalServerError)
	}))
	defer unreachable.Close()
	report, warnings = InspectA2AAuthentication(context.Background(), middleware.ProtocolEndpoint{
		Kind:    middleware.ProtocolKindA2A,
		Address: unreachable.URL,
	}, a2aclient.FetchRemoteAuthCard)
	if report == nil || report.Status != A2AAuthCardUnavailable || len(warnings) != 1 {
		t.Fatalf("an unreadable card must be a warning, got %+v warnings=%v", report, warnings)
	}
}
