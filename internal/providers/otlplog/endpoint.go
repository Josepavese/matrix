package otlplog

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func validateEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid collector endpoint")
	}
	if !safeEndpointFields(u) {
		return "", fmt.Errorf("collector endpoint must be an absolute URL without embedded credentials, query or fragment")
	}
	if !safeEndpointTransport(u) {
		return "", fmt.Errorf("collector requires HTTPS except for explicit loopback HTTP")
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/logs"
	}
	return u.String(), nil
}

func safeEndpointFields(u *url.URL) bool {
	return u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

func safeEndpointTransport(u *url.URL) bool {
	if u.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()))
}

func resolveAuthorization(resolve func() (string, error)) (string, error) {
	if resolve == nil {
		return "", nil
	}
	value, err := resolve()
	if err != nil {
		return "", fmt.Errorf("collector credential unavailable")
	}
	if strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("invalid collector authorization header")
	}
	return value, nil
}
