package zedacp

import (
	"fmt"
	"strings"
)

// ConfiguredProtocolVersion maps an operator's agent-level protocol pin to the
// initialize number. Empty means "negotiate the highest generation Matrix
// supports"; 1 pins the connection to version 1, which is the reversible
// rollback path for a peer whose version 2 payload is not conforming. Other
// values fail closed: a pin that silently degraded to another generation would
// not be a pin.
func ConfiguredProtocolVersion(raw string) (int, error) {
	switch strings.TrimSpace(raw) {
	case "":
		return 0, nil
	case "1", "1.0":
		return ProtocolVersionV1, nil
	case "2", "2.0":
		return ProtocolVersionV2, nil
	default:
		return 0, fmt.Errorf("unsupported configured ACP protocol version %q (want 1 or 2)", raw)
	}
}

// ValidateNegotiatedProtocolVersion fails closed when the connection agreed on a
// generation this client cannot speak.
func ValidateNegotiatedProtocolVersion(agreed int) error {
	if agreed < ProtocolVersionV1 || agreed > MaxSupportedProtocolVersion {
		return fmt.Errorf("ACP protocol version %d is not supported (matrix supports %d..%d)",
			agreed, ProtocolVersionV1, MaxSupportedProtocolVersion)
	}
	return nil
}

// ValidateInitializeResponse verifies the generation-specific payload after the
// version has been agreed. A version 2 peer must answer with the version 2
// parameter names: a peer that echoes "protocolVersion": 2 while sending the
// version 1 "agentCapabilities"/"agentInfo" payload is not a version 2 agent,
// and silently accepting it would advertise a surface the peer never agreed to.
func ValidateInitializeResponse(resp *InitializeResponse, agreed int) error {
	if err := ValidateNegotiatedProtocolVersion(agreed); err != nil {
		return err
	}
	if agreed >= ProtocolVersionV2 && (resp == nil || !resp.V2PayloadConforming) {
		return fmt.Errorf("ACP peer declared protocol version 2 but sent a non-conforming payload: info and capabilities are required")
	}
	return nil
}
