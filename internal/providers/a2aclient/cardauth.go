package a2aclient

import (
	"context"
	"sort"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// Remote authentication scheme kinds, mirroring the OpenAPI-style scheme types
// an A2A agent card declares.
const (
	RemoteAuthAPIKey        = "apiKey"
	RemoteAuthHTTP          = "http"
	RemoteAuthOAuth2        = "oauth2"
	RemoteAuthOpenIDConnect = "openIdConnect"
	RemoteAuthMutualTLS     = "mutualTLS"
	RemoteAuthUnknown       = "unknown"
)

// API key locations, mirroring the card's "in" field.
const (
	RemoteAuthInHeader = "header"
	RemoteAuthInQuery  = "query"
	RemoteAuthInCookie = "cookie"
)

// RemoteAuthScheme is one security scheme a remote agent card declares,
// normalized to plain Go types so callers never import the protocol SDK.
type RemoteAuthScheme struct {
	// Name is the card's scheme name: the key a security requirement references.
	Name string
	// Kind is the declared scheme type: apiKey, http, oauth2, openIdConnect,
	// mutualTLS, or unknown for a shape Matrix does not model.
	Kind string
	// In is where an apiKey travels: header, query or cookie.
	In string
	// Parameter is the apiKey header, query or cookie parameter name.
	Parameter string
	// Scheme is the http authentication scheme, such as bearer.
	Scheme string
}

// RemoteAuthRequirement is one alternative of the card's security list: every
// scheme it names must hold together (AND). Alternatives form a disjunction.
type RemoteAuthRequirement struct {
	Names []string
}

// RemoteAuthCard is the authentication a remote agent card declares. Schemes is
// sorted by name and each requirement's Names is sorted, so a report built from
// it is deterministic.
type RemoteAuthCard struct {
	Schemes      []RemoteAuthScheme
	Requirements []RemoteAuthRequirement
}

// FetchRemoteAuthCard resolves a remote agent card through the same resolver
// and configured headers the outbound client uses, and returns the
// authentication it declares.
func FetchRemoteAuthCard(ctx context.Context, cardURL string, headers map[string]string) (*RemoteAuthCard, error) {
	card, err := resolveAgentCard(ctx, cardURL, headers)
	if err != nil {
		return nil, err
	}
	return remoteAuthFromCard(card), nil
}

func remoteAuthFromCard(card *a2a.AgentCard) *RemoteAuthCard {
	out := &RemoteAuthCard{}
	if card == nil {
		return out
	}
	for name, scheme := range card.SecuritySchemes {
		out.Schemes = append(out.Schemes, remoteAuthScheme(string(name), scheme))
	}
	sort.Slice(out.Schemes, func(i, j int) bool { return out.Schemes[i].Name < out.Schemes[j].Name })
	for _, requirement := range card.SecurityRequirements {
		alternative := RemoteAuthRequirement{Names: make([]string, 0, len(requirement))}
		for name := range requirement {
			alternative.Names = append(alternative.Names, string(name))
		}
		sort.Strings(alternative.Names)
		out.Requirements = append(out.Requirements, alternative)
	}
	return out
}

func remoteAuthScheme(name string, scheme a2a.SecurityScheme) RemoteAuthScheme {
	out := RemoteAuthScheme{Name: name, Kind: RemoteAuthUnknown}
	switch typed := scheme.(type) {
	case a2a.APIKeySecurityScheme:
		out.Kind = RemoteAuthAPIKey
		out.In = strings.ToLower(strings.TrimSpace(string(typed.Location)))
		out.Parameter = strings.TrimSpace(typed.Name)
	case a2a.HTTPAuthSecurityScheme:
		out.Kind = RemoteAuthHTTP
		out.Scheme = strings.TrimSpace(typed.Scheme)
	case a2a.OAuth2SecurityScheme:
		out.Kind = RemoteAuthOAuth2
	case a2a.OpenIDConnectSecurityScheme:
		out.Kind = RemoteAuthOpenIDConnect
	case a2a.MutualTLSSecurityScheme:
		out.Kind = RemoteAuthMutualTLS
	}
	return out
}
