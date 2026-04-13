package osfs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/jose/matrix-v2/internal/middleware"
)

// Localizer is the OS implementation of the LocalizationReader interface.
type Localizer struct {
	baseDir  string
	fallback string
	cache    map[string]map[string]string // lang_code -> {key -> value}
	mu       sync.RWMutex
}

// NewLocalizer creates a new filesystem-backed Localizer.
func NewLocalizer(baseDir, fallbackLang string) middleware.LocalizationReader {
	return &Localizer{
		baseDir:  baseDir,
		fallback: fallbackLang,
		cache:    make(map[string]map[string]string),
	}
}

// loadTranslations load a specific language file into the cache.
func (l *Localizer) loadTranslations(langCode string) error {
	filePath := filepath.Join(l.baseDir, fmt.Sprintf("%s.json", langCode))
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var translations map[string]string
	if err := json.Unmarshal(data, &translations); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache[langCode] = translations
	return nil
}

// GetString implements the middleware.LocalizationReader interface.
func (l *Localizer) GetString(langCode, key string) string {
	l.mu.RLock()
	langData, cached := l.cache[langCode]
	l.mu.RUnlock()

	// If not cached, attempt to load
	if !cached {
		if err := l.loadTranslations(langCode); err != nil {
			// Fallback if load fails and we are not already trying the fallback
			if langCode != l.fallback {
				return l.GetString(l.fallback, key)
			}
			return key // Return key itself as last resort
		}

		l.mu.RLock()
		langData = l.cache[langCode]
		l.mu.RUnlock()
	}

	if val, ok := langData[key]; ok {
		return val
	}

	// Try fallback language if key not found in current language
	if langCode != l.fallback {
		return l.GetString(l.fallback, key)
	}

	// Ultimate fallback is the key string
	return key
}
