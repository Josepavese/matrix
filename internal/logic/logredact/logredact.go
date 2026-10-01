// Package logredact holds the redaction policy for what Matrix writes into its
// own logs without anyone asking for it. A value that says where a provider
// lives is withheld unless the operator reveals it explicitly, and header values
// are never written at all: a log is a record nobody requested, so the
// protective choice is the default and the exception must be declared.
//
// This policy governs logging only. A surface an operator queries on purpose
// (for example the doctor report) is an answer to a question, not a record
// written behind their back, and keeps showing what it was asked for.
package logredact

import (
	"os"
	"sort"
	"strings"
)

// Redacted is what a withheld value is replaced with.
const Redacted = "***"

// revealEndpointsEnv is the single opt-in for provider endpoints in logs. It
// follows the operator override convention the runtime already uses (for example
// MATRIX_ACP_V2_TURN_BUDGET): unset means the protective behaviour, and only the
// documented value turns it off.
const revealEndpointsEnv = "MATRIX_LOG_REVEAL_ENDPOINTS"

// RevealEndpoints reports whether the operator asked for provider endpoints to
// appear in logs.
func RevealEndpoints() bool {
	return strings.TrimSpace(os.Getenv(revealEndpointsEnv)) == "1"
}

// Endpoint returns what may be written to a log for a provider endpoint or host.
// The default withholds the whole value, so no host, scheme, path or user
// information reaches the log. An empty endpoint stays empty: there is nothing
// to withhold, and a placeholder would claim a target the record never had.
func Endpoint(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || RevealEndpoints() {
		return value
	}
	return Redacted
}

// HeaderNames returns the configured header names, sorted, and never their
// values. A caller that wants to say which authentication headers were supplied
// can log this and still keep every credential out of the record.
func HeaderNames(headers map[string]string) []string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	sort.Strings(names)
	return names
}
