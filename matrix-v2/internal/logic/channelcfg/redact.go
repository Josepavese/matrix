package channelcfg

import "strings"

func RedactMap(values map[string]string) map[string]string {
	redacted := make(map[string]string, len(values))
	for key, value := range values {
		if masked, ok := RedactValue(key, value); ok {
			redacted[key] = masked
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func RedactValue(key, value string) (string, bool) {
	if !isSecretKey(key) {
		return "", false
	}
	return RedactSecret(value), true
}

func RedactSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + "..." + value[len(value)-4:]
}

func IsSecretKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "key")
}

func isSecretKey(key string) bool {
	return IsSecretKey(key)
}
