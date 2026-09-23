package agentdoctor

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/a2aclient"
)

// A2A authentication preflight verdicts. They describe what a remote agent card
// declares and what the configured endpoint headers can be checked against;
// they never describe the outcome of a real call.
const (
	A2AAuthSatisfied       = "satisfied"
	A2AAuthMissing         = "missing"
	A2AAuthUnverifiable    = "unverifiable"
	A2AAuthNotDeclared     = "not_declared"
	A2AAuthCardUnavailable = "card_unavailable"
)

// a2aAuthFetchTimeout bounds the extra agent-card read doctor performs. It
// matches the 30s timeout of the A2A card resolver instead of adding a longer
// one.
const a2aAuthFetchTimeout = 30 * time.Second

// RemoteAuthFetcher reads the authentication a remote A2A endpoint declares.
// a2aclient.FetchRemoteAuthCard satisfies it: doctor reuses the resolver and
// the configured headers the outbound client already uses rather than opening a
// second HTTP client.
type RemoteAuthFetcher func(ctx context.Context, cardURL string, headers map[string]string) (*a2aclient.RemoteAuthCard, error)

// A2AAuthentication is doctor's preflight report for a remote A2A endpoint: the
// authentication its agent card declares and whether the endpoint's configured
// headers plausibly satisfy it. The judgement is a heuristic, not an
// enforcement point: nothing here blocks, rewrites or performs a call, and
// "satisfied" only means the expected header is present and shaped like the
// scheme's credential. Credential values are never reported.
type A2AAuthentication struct {
	CardURL           string                         `json:"card_url"`
	Status            string                         `json:"status"`
	Detail            string                         `json:"detail,omitempty"`
	ConfiguredHeaders []string                       `json:"configured_headers,omitempty"`
	DeclaredSchemes   []string                       `json:"declared_schemes,omitempty"`
	Requirements      []A2AAuthenticationRequirement `json:"requirements,omitempty"`
}

// A2AAuthenticationRequirement is one entry of the card's security list.
// Entries are alternatives (OR); the schemes inside one entry must all hold
// together (AND).
type A2AAuthenticationRequirement struct {
	Status  string                    `json:"status"`
	Schemes []A2AAuthenticationScheme `json:"schemes"`
}

// A2AAuthenticationScheme is one declared scheme judged against the configured
// headers. Only names and shapes are reported, never credential values.
type A2AAuthenticationScheme struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Location string `json:"location,omitempty"`
	Header   string `json:"header,omitempty"`
	Scheme   string `json:"scheme,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// InspectA2AAuthentication reads the agent card of an A2A endpoint and reports
// its authentication requirements. It reports nothing for endpoints that are
// not A2A or that carry neither a card URL nor an address, so doctor never
// probes a protocol it does not own. A card that cannot be read is a warning,
// never an error: the rest of the doctor report must still be produced.
func InspectA2AAuthentication(ctx context.Context, endpoint middleware.ProtocolEndpoint, fetch RemoteAuthFetcher) (*A2AAuthentication, []string) {
	if endpoint.Kind != middleware.ProtocolKindA2A || fetch == nil {
		return nil, nil
	}
	cardURL := a2aCardTarget(endpoint)
	if cardURL == "" {
		return nil, nil
	}
	fetchCtx, cancel := context.WithTimeout(ctx, a2aAuthFetchTimeout)
	defer cancel()
	card, err := fetch(fetchCtx, cardURL, endpoint.Headers)
	if err != nil {
		return a2aCardUnavailable(cardURL, err.Error()), []string{"could not read A2A agent card at " + cardURL + " for authentication preflight: " + err.Error()}
	}
	if card == nil {
		return a2aCardUnavailable(cardURL, "the card resolver returned no card"), []string{"could not read A2A agent card at " + cardURL + " for authentication preflight: the card resolver returned no card"}
	}
	report, warnings := EvaluateA2AAuthentication(card, endpoint.Headers)
	report.CardURL = cardURL
	return report, warnings
}

func a2aCardUnavailable(cardURL, detail string) *A2AAuthentication {
	return &A2AAuthentication{CardURL: cardURL, Status: A2AAuthCardUnavailable, Detail: detail}
}

// a2aCardTarget is the URL doctor reads the card from: the configured card URL
// when there is one, otherwise the endpoint address (the resolver then appends
// the standard well-known agent-card path). As on the serving side, an A2A
// endpoint may also carry its target in the command field.
func a2aCardTarget(endpoint middleware.ProtocolEndpoint) string {
	for _, candidate := range []string{endpoint.CardURL, endpoint.Address, endpoint.Command} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// EvaluateA2AAuthentication judges a declared agent-card contract against the
// endpoint's configured headers. The judgement is deliberately heuristic:
// apiKey and bearer credentials are recognised by name and shape alone, so a
// present but wrong value still reads as satisfied, and anything negotiated
// outside the headers (oauth2, OpenID Connect, mutual TLS, non-bearer HTTP
// auth, a query or cookie apiKey) is reported as not verifiable rather than
// guessed at.
func EvaluateA2AAuthentication(card *a2aclient.RemoteAuthCard, headers map[string]string) (*A2AAuthentication, []string) {
	if card == nil {
		return &A2AAuthentication{Status: A2AAuthCardUnavailable, Detail: "no agent card to inspect"}, nil
	}
	report := &A2AAuthentication{ConfiguredHeaders: headerNames(headers)}
	for _, scheme := range card.Schemes {
		report.DeclaredSchemes = append(report.DeclaredSchemes, scheme.Name)
	}
	if len(card.Requirements) == 0 {
		report.Status = A2AAuthNotDeclared
		report.Detail = "the remote agent card declares no security requirements"
		if len(report.DeclaredSchemes) > 0 {
			report.Detail = "the remote agent card declares security schemes but no security requirements; calls are not required to authenticate"
		}
		return report, nil
	}
	declared := make(map[string]a2aclient.RemoteAuthScheme, len(card.Schemes))
	for _, scheme := range card.Schemes {
		declared[scheme.Name] = scheme
	}
	for _, requirement := range card.Requirements {
		report.Requirements = append(report.Requirements, evaluateRequirement(requirement, declared, headers))
	}
	report.Status, report.Detail = summarizeRequirements(report.Requirements)
	if report.Status != A2AAuthMissing {
		return report, nil
	}
	return report, []string{missingCredentialWarning(report.Requirements)}
}

func evaluateRequirement(requirement a2aclient.RemoteAuthRequirement, declared map[string]a2aclient.RemoteAuthScheme, headers map[string]string) A2AAuthenticationRequirement {
	out := A2AAuthenticationRequirement{Schemes: make([]A2AAuthenticationScheme, 0, len(requirement.Names))}
	for _, name := range requirement.Names {
		scheme, ok := declared[name]
		if !ok {
			out.Schemes = append(out.Schemes, A2AAuthenticationScheme{Name: name, Type: a2aclient.RemoteAuthUnknown, Status: A2AAuthUnverifiable, Detail: fmt.Sprintf("agent card does not define security scheme %q", name)})
			continue
		}
		out.Schemes = append(out.Schemes, evaluateScheme(scheme, headers))
	}
	out.Status = requirementStatus(out.Schemes)
	return out
}

// requirementStatus combines the schemes of one requirement: an entry is
// satisfied only when every scheme in it is (an empty entry means the card
// allows anonymous access), missing as soon as one scheme is missing, and
// otherwise not verifiable.
func requirementStatus(schemes []A2AAuthenticationScheme) string {
	status := A2AAuthSatisfied
	for _, scheme := range schemes {
		if scheme.Status == A2AAuthMissing {
			return A2AAuthMissing
		}
		if scheme.Status == A2AAuthUnverifiable {
			status = A2AAuthUnverifiable
		}
	}
	return status
}

func summarizeRequirements(requirements []A2AAuthenticationRequirement) (string, string) {
	satisfied, unverifiable := false, false
	for _, requirement := range requirements {
		switch requirement.Status {
		case A2AAuthSatisfied:
			satisfied = true
		case A2AAuthUnverifiable:
			unverifiable = true
		}
	}
	if satisfied {
		return A2AAuthSatisfied, "the configured headers appear to satisfy one of the card's security requirements"
	}
	if unverifiable {
		return A2AAuthUnverifiable, "the card's security requirements cannot be verified from configured headers"
	}
	return A2AAuthMissing, "no configured header satisfies any of the card's security requirements"
}

// missingCredentialWarning names what is absent so the operator can act on it.
// Only unsatisfiable alternatives are reported: an alternative that holds, or
// that cannot be judged, is not a missing credential.
func missingCredentialWarning(requirements []A2AAuthenticationRequirement) string {
	var missing, unverifiable []string
	for _, requirement := range requirements {
		if requirement.Status != A2AAuthMissing {
			continue
		}
		for _, scheme := range requirement.Schemes {
			switch scheme.Status {
			case A2AAuthMissing:
				missing = append(missing, missingSchemeCredential(scheme))
			case A2AAuthUnverifiable:
				unverifiable = append(unverifiable, fmt.Sprintf("%s scheme %q", scheme.Type, scheme.Name))
			}
		}
	}
	warning := "A2A agent card requires authentication the configured endpoint headers do not appear to provide: " + strings.Join(sortedUnique(missing), "; ")
	if len(unverifiable) > 0 {
		warning += "; not verifiable from headers: " + strings.Join(sortedUnique(unverifiable), ", ")
	}
	return warning
}

func missingSchemeCredential(scheme A2AAuthenticationScheme) string {
	if scheme.Type == a2aclient.RemoteAuthAPIKey {
		return fmt.Sprintf("apiKey scheme %q needs header %s", scheme.Name, scheme.Header)
	}
	if scheme.Type == a2aclient.RemoteAuthHTTP {
		return fmt.Sprintf("http %q scheme %q needs an Authorization header whose value starts with %q", scheme.Scheme, scheme.Name, "Bearer ")
	}
	return fmt.Sprintf("%s scheme %q is not satisfied", scheme.Type, scheme.Name)
}

func evaluateScheme(scheme a2aclient.RemoteAuthScheme, headers map[string]string) A2AAuthenticationScheme {
	out := A2AAuthenticationScheme{Name: scheme.Name, Type: scheme.Kind, Status: A2AAuthUnverifiable}
	switch scheme.Kind {
	case a2aclient.RemoteAuthAPIKey:
		out.Location, out.Header = scheme.In, scheme.Parameter
		return judgeAPIKeyScheme(out, headers)
	case a2aclient.RemoteAuthHTTP:
		out.Scheme = scheme.Scheme
		return judgeHTTPScheme(out, headers)
	case a2aclient.RemoteAuthOAuth2:
		out.Detail = "oauth2 credentials are obtained out of band and cannot be verified from configured headers"
	case a2aclient.RemoteAuthOpenIDConnect:
		out.Detail = "OpenID Connect tokens are obtained out of band and cannot be verified from configured headers"
	case a2aclient.RemoteAuthMutualTLS:
		out.Detail = "mutual TLS is carried by the connection's client certificate, not by configured headers"
	default:
		out.Type = a2aclient.RemoteAuthUnknown
		out.Detail = "a security scheme Matrix does not model cannot be verified from configured headers"
	}
	return out
}

func judgeAPIKeyScheme(out A2AAuthenticationScheme, headers map[string]string) A2AAuthenticationScheme {
	if out.Location != a2aclient.RemoteAuthInHeader {
		out.Detail = fmt.Sprintf("apiKey is carried in %q, which configured request headers cannot satisfy or disprove", out.Location)
		return out
	}
	if out.Header == "" {
		out.Detail = "agent card declares an apiKey header scheme without a header name"
		return out
	}
	value, ok := headerValue(headers, out.Header)
	if !ok {
		out.Status = A2AAuthMissing
		out.Detail = fmt.Sprintf("header %s is not configured for this endpoint", out.Header)
		return out
	}
	if strings.TrimSpace(value) == "" {
		out.Status = A2AAuthMissing
		out.Detail = fmt.Sprintf("header %s is configured but empty", out.Header)
		return out
	}
	out.Status = A2AAuthSatisfied
	out.Detail = fmt.Sprintf("header %s is configured", out.Header)
	return out
}

func judgeHTTPScheme(out A2AAuthenticationScheme, headers map[string]string) A2AAuthenticationScheme {
	if !strings.EqualFold(strings.TrimSpace(out.Scheme), "bearer") {
		out.Detail = fmt.Sprintf("http %q credentials cannot be verified from configured headers", out.Scheme)
		return out
	}
	value, ok := headerValue(headers, "Authorization")
	if !ok {
		out.Status = A2AAuthMissing
		out.Detail = `an Authorization header whose value starts with "Bearer " is required`
		return out
	}
	if !isBearerValue(value) {
		out.Status = A2AAuthMissing
		out.Detail = `the configured Authorization header does not start with "Bearer "`
		return out
	}
	out.Status = A2AAuthSatisfied
	out.Detail = "an Authorization header with a Bearer credential is configured"
	return out
}

func isBearerValue(value string) bool {
	const prefix = "bearer "
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < len(prefix) || !strings.EqualFold(trimmed[:len(prefix)], prefix) {
		return false
	}
	return strings.TrimSpace(trimmed[len(prefix):]) != ""
}

// headerValue matches header names case-insensitively, as HTTP requires.
func headerValue(headers map[string]string, name string) (string, bool) {
	for key, value := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			return value, true
		}
	}
	return "", false
}

// headerNames lists configured header names for display. Values are never
// reported: doctor output is expected to be pasted into issues.
func headerNames(headers map[string]string) []string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	sort.Strings(names)
	return names
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	out := make([]string, 0, len(values))
	for index, value := range values {
		if index == 0 || value != values[index-1] {
			out = append(out, value)
		}
	}
	return out
}
