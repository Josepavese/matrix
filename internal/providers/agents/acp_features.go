package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

type acpFeatureCapabilities struct {
	promptImage           bool
	promptAudio           bool
	promptEmbeddedContext bool
	mcpHTTP               bool
	mcpSSE                bool
	logout                bool
	fsRead                bool
	fsWrite               bool
	terminal              bool
}

func parseACPFeatureCapabilities(resp *acpInitializeResponse) acpFeatureCapabilities {
	if resp == nil {
		return acpFeatureCapabilities{}
	}
	prompt, _ := resp.Capabilities["promptCapabilities"].(map[string]interface{})
	mcp, _ := resp.Capabilities["mcpCapabilities"].(map[string]interface{})
	auth, _ := resp.Capabilities["auth"].(map[string]interface{})
	return acpFeatureCapabilities{
		promptImage:           boolCapability(prompt["image"]),
		promptAudio:           boolCapability(prompt["audio"]),
		promptEmbeddedContext: boolCapability(prompt["embeddedContext"]),
		mcpHTTP:               boolCapability(mcp["http"]),
		mcpSSE:                boolCapability(mcp["sse"]),
		logout:                capabilityEnabled(auth["logout"]),
	}
}

func boolCapability(value interface{}) bool {
	enabled, _ := value.(bool)
	return enabled
}

func (c *acpConversationClient) validatePromptContent(blocks []middleware.Content) error {
	for _, block := range blocks {
		switch strings.ToLower(strings.TrimSpace(block.Type)) {
		case "text", "resource_link":
		case "image":
			if !c.featureCapabilities.promptImage {
				return fmt.Errorf("ACP agent does not advertise promptCapabilities.image")
			}
		case "audio":
			if !c.featureCapabilities.promptAudio {
				return fmt.Errorf("ACP agent does not advertise promptCapabilities.audio")
			}
		case "resource":
			if !c.featureCapabilities.promptEmbeddedContext {
				return fmt.Errorf("ACP agent does not advertise promptCapabilities.embeddedContext")
			}
		default:
			return fmt.Errorf("content type %q is not part of stable ACP v1", block.Type)
		}
	}
	return nil
}

func (c *acpConversationClient) validateMCPServers(servers []acpMcpServerConfig) error {
	for _, server := range servers {
		switch strings.ToLower(strings.TrimSpace(server.Type)) {
		case "", "stdio":
		case "http":
			if !c.featureCapabilities.mcpHTTP {
				return fmt.Errorf("ACP agent does not advertise mcpCapabilities.http")
			}
		case "sse":
			if !c.featureCapabilities.mcpSSE {
				return fmt.Errorf("ACP agent does not advertise mcpCapabilities.sse")
			}
		default:
			return fmt.Errorf("MCP transport %q is not part of stable ACP v1", server.Type)
		}
	}
	return nil
}

// AuthenticationMethods, Authenticate and the rest of the ACP v2 authentication
// surface live in acp_authentication.go; this file keeps the capability and
// content validation the adapter performs before a turn.

func (c *acpConversationClient) Logout(ctx context.Context) error {
	if !c.featureCapabilities.logout {
		return fmt.Errorf("ACP agent does not advertise auth.logout")
	}
	_, err := c.currentACPClient().Logout(ctx, acpLogoutRequest{})
	return err
}

var _ middleware.ConversationAuthenticationControl = (*acpConversationClient)(nil)
