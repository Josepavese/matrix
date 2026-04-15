package middleware

// LocalizationReader defines the interface for retrieving localized strings.
// This enforces SSOT (externalizing strings) and PAL (abstracting the source of translations).
type LocalizationReader interface {
	// GetString retrieves a localized string for the given language and key.
	// Defaults to a fallback language or the key itself if not found.
	GetString(langCode, key string) string
}
