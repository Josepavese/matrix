package osfs

import "os"

// ConfigProvider implements middleware.ConfigReader using the native OS filesystem.
// This is the platform-specific provider for config reading.
type ConfigProvider struct{}

// NewConfigProvider creates a new ConfigProvider.
func NewConfigProvider() *ConfigProvider {
	return &ConfigProvider{}
}

func (p *ConfigProvider) ReadConfig(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (p *ConfigProvider) WriteConfig(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
