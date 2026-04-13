package channelcfg

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/config"
)

func ProviderKey(provider, key string) (string, error) {
	provider = strings.TrimSpace(provider)
	key = strings.TrimSpace(key)
	if provider == "" || key == "" || strings.Contains(key, " ") {
		return "", fmt.Errorf("invalid channel key")
	}
	if _, err := providerDefinitionFor(provider); err != nil {
		return "", err
	}
	if !supportsProviderKey(provider, key) {
		return "", fmt.Errorf("unsupported key %q for channel provider %q", key, provider)
	}
	return "channel." + provider + "." + key, nil
}

func SetOverride(cfgMgr *config.Manager, provider, key, value string) error {
	fullKey, err := ProviderKey(provider, key)
	if err != nil {
		return err
	}
	return cfgMgr.Set(fullKey, value)
}

func DeleteOverride(cfgMgr *config.Manager, provider, key string) error {
	fullKey, err := ProviderKey(provider, key)
	if err != nil {
		return err
	}
	return cfgMgr.Delete(fullKey)
}

func ListOverrides(cfgMgr *config.Manager, provider string) (map[string]string, error) {
	if _, err := providerDefinitionFor(provider); err != nil {
		return nil, err
	}
	keys, err := cfgMgr.List()
	if err != nil {
		return nil, err
	}
	prefix := "channel." + provider + "."
	values := map[string]string{}
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		value, err := cfgMgr.Get(key)
		if err != nil {
			return nil, err
		}
		values[strings.TrimPrefix(key, prefix)] = value
	}
	return values, nil
}

func SortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
