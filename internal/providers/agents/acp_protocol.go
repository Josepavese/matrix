package agents

import (
	"fmt"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

const supportedACPProtocolVersion = 1

// validateNegotiatedProtocolVersion fails closed when the connection agreed on a
// generation Matrix cannot speak. It reads the version the client recorded while
// negotiating rather than the raw response field: a v1 agent may omit the field
// entirely, and an agent that declares nothing is a v1 agent, not an unsupported
// one.
func validateNegotiatedProtocolVersion(client acpClient) error {
	agreed := client.AuthenticatedProtocolVersion()
	if agreed < supportedACPProtocolVersion || agreed > zedacp.MaxSupportedProtocolVersion {
		return fmt.Errorf("ACP protocol version %d is not supported (matrix supports %d..%d)",
			agreed, supportedACPProtocolVersion, zedacp.MaxSupportedProtocolVersion)
	}
	return nil
}

// negotiatedProtocolVersion reports the generation this connection speaks. Zero
// means initialize has not completed, which is never version 2.
func (c *acpConversationClient) negotiatedProtocolVersion() int {
	client := c.currentACPClient()
	if client == nil {
		return 0
	}
	return client.AuthenticatedProtocolVersion()
}

// terminalAuthenticationOffered reports whether a terminal method may be offered
// on this connection: the agent agreed to version 2, where the type exists, and
// Matrix both opted in and has a process backend to run it with.
