package runtimecheck

import (
	"fmt"
	"net"
	"strings"
)

func RequireAPIKeyForExternalBind(addr, apiKey, addrKey, apiKeyName string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid %s %q: %w", addrKey, addr, err)
	}
	host = strings.TrimSpace(host)
	if isLoopbackBindHost(host) || strings.TrimSpace(apiKey) != "" {
		return nil
	}
	return fmt.Errorf("%s=%q is not loopback; set %s before exposing Matrix outside localhost", addrKey, addr, apiKeyName)
}

func isLoopbackBindHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
