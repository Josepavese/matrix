package a2aclient

import (
	"context"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// TestRemoteAuthFromCardNormalizesEveryDeclaredShape pins the SDK-to-neutral
// mapping: callers outside this package must receive the card's authentication
// as plain Go types, without importing the protocol SDK.
func TestRemoteAuthFromCardNormalizesEveryDeclaredShape(t *testing.T) {
	card := &a2a.AgentCard{
		SecuritySchemes: a2a.NamedSecuritySchemes{
			"key":    a2a.APIKeySecurityScheme{Name: " X-Peer-Key ", Location: a2a.APIKeySecuritySchemeLocationHeader},
			"bearer": a2a.HTTPAuthSecurityScheme{Scheme: " bearer ", BearerFormat: "JWT"},
			"sso":    a2a.OAuth2SecurityScheme{Flows: a2a.ClientCredentialsOAuthFlow{TokenURL: "https://peer.example/token"}},
			"oidc":   a2a.OpenIDConnectSecurityScheme{OpenIDConnectURL: "https://peer.example/.well-known/openid-configuration"},
			"mtls":   a2a.MutualTLSSecurityScheme{Description: "client certificate"},
		},
		SecurityRequirements: a2a.SecurityRequirementsOptions{
			a2a.SecurityRequirements{"key": {}},
			a2a.SecurityRequirements{"mtls": {}, "bearer": {}},
		},
	}

	got := remoteAuthFromCard(card)
	want := map[string]RemoteAuthScheme{
		"key":    {Name: "key", Kind: RemoteAuthAPIKey, In: RemoteAuthInHeader, Parameter: "X-Peer-Key"},
		"bearer": {Name: "bearer", Kind: RemoteAuthHTTP, Scheme: "bearer"},
		"sso":    {Name: "sso", Kind: RemoteAuthOAuth2},
		"oidc":   {Name: "oidc", Kind: RemoteAuthOpenIDConnect},
		"mtls":   {Name: "mtls", Kind: RemoteAuthMutualTLS},
	}
	if len(got.Schemes) != len(want) {
		t.Fatalf("every declared scheme must be normalized, got %+v", got.Schemes)
	}
	for index, scheme := range got.Schemes {
		if scheme != want[scheme.Name] {
			t.Fatalf("scheme %d (%q) normalized to %+v", index, scheme.Name, scheme)
		}
		if index > 0 && got.Schemes[index-1].Name > scheme.Name {
			t.Fatalf("schemes must be sorted for a deterministic report, got %+v", got.Schemes)
		}
	}
	if len(got.Requirements) != 2 {
		t.Fatalf("both security alternatives must be kept, got %+v", got.Requirements)
	}
	if names := got.Requirements[1].Names; len(names) != 2 || names[0] != "bearer" || names[1] != "mtls" {
		t.Fatalf("a requirement must name its schemes in sorted order, got %v", names)
	}
}

// TestRemoteAuthFromCardHandlesADegenerateCard keeps an absent or empty card
// from panicking an inspector and from inventing a requirement.
func TestRemoteAuthFromCardHandlesADegenerateCard(t *testing.T) {
	for name, card := range map[string]*a2a.AgentCard{
		"nil card":   nil,
		"empty card": {},
		"nil scheme": {SecuritySchemes: a2a.NamedSecuritySchemes{"ghost": nil}},
	} {
		t.Run(name, func(t *testing.T) {
			got := remoteAuthFromCard(card)
			if got == nil {
				t.Fatal("normalization must always return a card")
			}
			if len(got.Requirements) != 0 {
				t.Fatalf("a card with no requirements must declare none, got %+v", got.Requirements)
			}
			if name == "nil scheme" && (len(got.Schemes) != 1 || got.Schemes[0].Kind != RemoteAuthUnknown) {
				t.Fatalf("a scheme Matrix cannot model must be reported as unknown, got %+v", got.Schemes)
			}
		})
	}
}

// TestFetchRemoteAuthCardRefusesAnUnusableSource keeps the fetch boundary
// honest: an invalid card URL must fail before any request is attempted.
func TestFetchRemoteAuthCardRefusesAnUnusableSource(t *testing.T) {
	if _, err := FetchRemoteAuthCard(context.Background(), "not a url", nil); err == nil {
		t.Fatal("an invalid card URL must be refused")
	}
}
