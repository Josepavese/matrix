package agentdoctor

import (
	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
)

func sandboxProbeReport(endpoint middleware.ProtocolEndpoint) (bool, map[string]any, []string) {
	policy, err := agentlaunch.ReadSandbox(endpoint)
	if err != nil {
		return true, map[string]any{"provider_status": "launch_policy_invalid"}, []string{err.Error()}
	}
	if policy != nil && policy.Container != nil {
		return true, map[string]any{"provider_status": "sandbox_requires_workspace_probe", "command_probe_ok": false, "provider_handshake_ok": false}, []string{"container command is in the image; use matrix sandbox doctor --workspace for prerequisite checks"}
	}
	return false, nil, nil
}
