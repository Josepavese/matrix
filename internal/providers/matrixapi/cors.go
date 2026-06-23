package matrixapi

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	corsAllowMethods = "GET, POST, OPTIONS"
	corsAllowHeaders = "Content-Type, X-Matrix-Key, Authorization"
)

// LocalCORSHandler allows browser clients served from local loopback origins to
// call Matrix's local HTTP API without opening the API to remote web origins.
func LocalCORSHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		if !isLoopbackHTTPOrigin(origin) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		setLocalCORSHeaders(w, origin)
		if r.Method == http.MethodOptions && strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")) != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func setLocalCORSHeaders(w http.ResponseWriter, origin string) {
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
	w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
	appendVary(w.Header(), "Origin")
}

func appendVary(header http.Header, value string) {
	current := header.Values("Vary")
	for _, entry := range current {
		for _, part := range strings.Split(entry, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}

func isLoopbackHTTPOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
