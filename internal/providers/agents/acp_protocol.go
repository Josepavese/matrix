package agents

import (
	"github.com/Josepavese/matrix/pkg/zedacp"
)

const supportedACPProtocolVersion = zedacp.ProtocolVersionV1

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
