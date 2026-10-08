package agents

import (
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentlaunch"
	"github.com/Josepavese/matrix/internal/middleware"
)

func prepareSandboxClient(endpoint middleware.ProtocolEndpoint, deps middleware.ConversationFactoryDeps) (middleware.ProtocolEndpoint, middleware.ConversationFactoryDeps, error) {
	policy, err := agentlaunch.ReadSandbox(endpoint)
	if err != nil || policy == nil {
		return endpoint, deps, err
	}
	resolved, err := agentlaunch.ResolveEndpoint(deps.AgentID, endpoint)
	if err != nil {
		return endpoint, deps, err
	}
	endpoint = resolved.Endpoint
	// Host ACP filesystem, terminals and terminal authentication cannot escape
	// the configured boundary. Providers use their own tools inside the image.
	deps.FS, deps.Process, deps.TerminalAuth = nil, nil, false
	deps.TrustMode = func() bool { return false }
	if len(deps.McpServers) > 0 {
		return endpoint, deps, fmt.Errorf("sandbox host MCP servers are not configured")
	}
	return endpoint, deps, nil
}

func (c *acpConversationClient) containerSandbox() bool {
	return c.endpoint.Sandbox != nil && c.endpoint.Sandbox.Container != nil
}

func (c *acpConversationClient) sandboxWorkspace(path string) (string, error) {
	if !c.containerSandbox() {
		return path, nil
	}
	if path != "" && !sameDirectory(path, c.cwd) {
		return "", fmt.Errorf("sandbox workspace differs from the mounted workspace")
	}
	return "/workspace", nil
}

func (c *acpConversationClient) validateSandboxTurn(turn middleware.ConversationTurn) error {
	if c.endpoint.Sandbox == nil {
		return nil
	}
	if _, err := c.sandboxWorkspace(strings.TrimSpace(turn.WorkspacePath)); err != nil {
		return err
	}
	if len(turn.Tools) > 0 {
		return fmt.Errorf("sandbox host extension tools are not configured")
	}
	if len(turn.McpServers) > 0 {
		return fmt.Errorf("sandbox MCP servers require explicit boundary configuration")
	}
	if len(turn.AdditionalDirectories) > 0 {
		return fmt.Errorf("sandbox additional directories require explicit mounts")
	}
	return nil
}

func (c *acpConversationClient) reportSandboxExecution(result *middleware.ConversationResult) {
	c.mu.Lock()
	evidence := c.sandboxEvidence
	c.mu.Unlock()
	if evidence == nil {
		return
	}
	if result.Metadata.Meta == nil {
		result.Metadata.Meta = map[string]interface{}{}
	}
	result.Metadata.Meta["sandbox_execution"] = *evidence
}

func (c *acpConversationClient) recordSandboxEvidence(transport middleware.AgentTransport) {
	if reporter, ok := transport.(middleware.SandboxExecutionReporter); ok {
		evidence := reporter.SandboxExecutionEvidence()
		c.mu.Lock()
		c.sandboxEvidence = &evidence
		c.mu.Unlock()
	}
}
