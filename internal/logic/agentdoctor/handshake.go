package agentdoctor

import (
	"context"
	"time"

	"github.com/Josepavese/matrix/internal/logic/providerfailure"
	"github.com/Josepavese/matrix/internal/middleware"
)

// HandshakeProbe runs the provider handshake and returns the protocol
// generation the peer agreed to, or zero when the protocol has no comparable
// generation. The doctor reports that number instead of inferring it.
type HandshakeProbe func(context.Context, middleware.ProtocolEndpoint) (int, error)

func ProbeHandshake(endpoint middleware.ProtocolEndpoint, probe HandshakeProbe) map[string]any {
	result := map[string]any{"provider_handshake_ok": false, "provider_status": "not_probed"}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	version, err := probe(ctx, endpoint)
	cancel()
	if err == nil {
		result["provider_handshake_ok"] = true
		result["provider_status"] = "ready_on_demand"
		if version > 0 {
			result["provider_protocol_version"] = version
		}
		return result
	}
	result["provider_status"] = "initialize_failed"
	result["provider_handshake_error"] = err.Error()
	if failure, ok := providerfailure.As(err); ok {
		result["provider_handshake_diagnostics"] = providerfailure.Details(failure)
	}
	return result
}
