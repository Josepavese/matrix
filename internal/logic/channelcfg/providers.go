// Package channelcfg manages per-channel configuration stored in the SSOT vault.
package channelcfg

import (
	"fmt"
	"sort"

	"github.com/Josepavese/matrix/internal/logic/config"
	"github.com/Josepavese/matrix/internal/middleware"
)

// ProviderState describes the configuration state of a channel provider.
type ProviderState struct {
	Provider  string            `json:"provider"`
	Keys      []string          `json:"keys"`
	Overrides map[string]string `json:"overrides"`
	Effective map[string]any    `json:"effective,omitempty"`
	Source    string            `json:"source,omitempty"`
}

type providerDefinition struct {
	Name string
	Keys []string
	Load func(reader middleware.ConfigReader, cfgMgr *config.Manager) (map[string]any, string, error)
}

var providerDefinitions = map[string]providerDefinition{
	"telegram": {
		Name: "telegram",
		Keys: []string{"admins", "enabled", "token"},
		Load: loadTelegramState,
	},
}

// SupportedProviders returns the names of all supported channel providers.
func SupportedProviders() []string {
	names := make([]string, 0, len(providerDefinitions))
	for name := range providerDefinitions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DescribeProvider returns the effective configuration for a provider.
func DescribeProvider(reader middleware.ConfigReader, cfgMgr *config.Manager, provider string) (ProviderState, error) {
	def, err := providerDefinitionFor(provider)
	if err != nil {
		return ProviderState{}, err
	}
	overrides, err := ListOverrides(cfgMgr, provider)
	if err != nil {
		return ProviderState{}, err
	}

	state := ProviderState{
		Provider:  def.Name,
		Keys:      append([]string{}, def.Keys...),
		Overrides: RedactMap(overrides),
	}
	if def.Load == nil {
		return state, nil
	}

	effective, source, err := def.Load(reader, cfgMgr)
	if err != nil {
		return ProviderState{}, err
	}
	state.Effective = effective
	state.Source = source
	return state, nil
}

func providerDefinitionFor(provider string) (providerDefinition, error) {
	def, ok := providerDefinitions[provider]
	if !ok {
		return providerDefinition{}, fmt.Errorf("unsupported channel provider %q", provider)
	}
	return def, nil
}

func supportsProviderKey(provider, key string) bool {
	def, ok := providerDefinitions[provider]
	if !ok {
		return false
	}
	for _, supported := range def.Keys {
		if supported == key {
			return true
		}
	}
	return false
}

func loadTelegramState(reader middleware.ConfigReader, cfgMgr *config.Manager) (map[string]any, string, error) {
	cfg, source, err := LoadTelegramConfig(reader, cfgMgr)
	if err != nil {
		return nil, "", err
	}
	return map[string]any{
		"token":   RedactSecret(cfg.Token),
		"enabled": cfg.Enabled,
		"admins":  cfg.Admins,
	}, source, nil
}
