package agentdoctor

import (
	"github.com/Josepavese/matrix/internal/middleware"
)

// EndpointAddress reports the effective connection target shown by doctor.
func EndpointAddress(endpoint middleware.ProtocolEndpoint) string {
	if endpoint.Kind == middleware.ProtocolKindACP && endpoint.Transport == "stdio" {
		return endpoint.Command
	}
	return endpoint.Address
}

// InspectACP performs command and initialize checks for a local ACP endpoint.
func InspectACP(endpoint middleware.ProtocolEndpoint, handshake HandshakeProbe) (map[string]any, []string) {
	result := map[string]any{}
	if endpoint.Kind != middleware.ProtocolKindACP || endpoint.Transport != "stdio" || endpoint.Command == "" {
		return result, nil
	}
	probe := ProbeCommand(endpoint.Command, endpoint.Args, endpoint.Env, endpoint.EnvIsolation)
	result["command_probe_ok"] = probe.OK
	result["command_probe_exit_code"] = probe.ExitCode
	if probe.Error != "" {
		result["command_probe_error"] = probe.Error
	}
	for key, value := range ProbeHandshake(endpoint, handshake) {
		result[key] = value
	}
	warnings := []string{}
	handshakeOK, _ := result["provider_handshake_ok"].(bool)
	if !probe.OK && !handshakeOK {
		warnings = append(warnings, "ACP stdio command probe failed")
	}
	if !handshakeOK {
		warnings = append(warnings, "ACP stdio initialize handshake failed")
	}
	return result, warnings
}
