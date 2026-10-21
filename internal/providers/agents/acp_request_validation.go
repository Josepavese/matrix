package agents

import (
	"encoding/json"
	"github.com/Josepavese/matrix/pkg/zedacp"
)

func (h *defaultRequestHandler) validateClientRequest(method string, params json.RawMessage) error {
	version := int(h.protocolVersion.Load())
	if version == 0 {
		version = supportedACPProtocolVersion
	}
	return zedacp.ValidateClientRequest(method, params, version)
}
