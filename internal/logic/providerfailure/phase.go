package providerfailure

// DefaultClassification distinguishes setup from an error returned while a
// provider is executing a turn. A coded RPC error preserves its diagnostics;
// its prose never becomes a quota category or an inferred reset timestamp.
func DefaultClassification(phase string, err error) (string, string) {
	if phase != "session/prompt" {
		return PreflightFailed, "agent provider preflight failed"
	}
	if isRPCError(err) {
		return APIError, "provider API failed during the turn"
	}
	return RuntimeFailed, "provider failed during the turn"
}

// EventKind reports when the failure occurred even for a specific error such
// as authentication or model access. These can fail during a running turn too.
func (e *Failure) EventKind() string {
	if e.Phase == "session/prompt" {
		return "provider.runtime.failed"
	}
	return "provider.preflight.failed"
}
