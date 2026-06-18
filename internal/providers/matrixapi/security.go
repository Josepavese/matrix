package matrixapi

import (
	"mime"
	"net/http"
	"strings"
)

func requireAPIKey(w http.ResponseWriter, r *http.Request, apiKey string) bool {
	if apiKey == "" {
		return true
	}
	if requestAPIKey(r) != apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func requestAPIKey(r *http.Request) string {
	if key := strings.TrimSpace(r.Header.Get("X-Matrix-Key")); key != "" {
		return key
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("bearer "):])
	}
	return ""
}

func requireJSONContentType(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Unsupported Media Type: application/json required", http.StatusUnsupportedMediaType)
		return false
	}
	return true
}
