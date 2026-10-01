// Package logredact holds the redaction policy for what Matrix writes into its
// own logs without anyone asking for it. A value that says where a provider
// lives is withheld unless the operator reveals it explicitly: a log is a record
// nobody requested, so the protective choice is the default and the exception
// must be declared.
//
// This policy governs logging only. A surface an operator queries on purpose
// (for example the doctor report) is an answer to a question, not a record
// written behind their back, and keeps showing what it was asked for.
//
// Header values are deliberately absent from this package. The guarantee is that
// no log site writes one, and that is a negative a helper cannot provide: an
// earlier HeaderNames helper was removed rather than left unreachable, because a
// promise no code keeps is worse than no promise. The one place that lists header
// names is the agent configuration display, and it is not a log.
package logredact

import (
	"os"
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
