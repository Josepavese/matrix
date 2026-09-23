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

// UnauthenticatedLoopbackWarning explains the exposure an operator accepts by running
// the ingress on loopback with no API key, and returns "" when the bind is safe or a
// key is set.
//
// Why a warning instead of a refusal: a non-loopback bind without a key is already
// refused, so requiring a key on loopback would break every existing install - and the
// operator's own runtime - to protect against local processes. What was missing was not
// a check but a voice: the shipped default is unauthenticated, and nothing said so. An
// operator who later puts a reverse proxy in front of this port has no signal that the
// proxy is now the only thing standing between the internet and an agent that can
// install software and write configuration.
func UnauthenticatedLoopbackWarning(addr, apiKey, addrKey, apiKeyName string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || strings.TrimSpace(apiKey) != "" || !isLoopbackBindHost(strings.TrimSpace(host)) {
		return ""
	}
	return fmt.Sprintf("%s=%q accepts unauthenticated local requests because %s is empty; "+
		"any local process, and anything you later put in front of this port, can drive agents, "+
		"install them and write configuration - set %s to require a key",
		addrKey, addr, apiKeyName, apiKeyName)
}
