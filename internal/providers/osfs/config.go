package osfs

import "os"

// ConfigProvider implements middleware.ConfigReader using the native OS filesystem.
// This is the platform-specific provider for config reading.
type ConfigProvider struct{}

// NewConfigProvider creates a new ConfigProvider.
func NewConfigProvider() *ConfigProvider {
	return &ConfigProvider{}
}

// ReadConfig reads the file at the given path and returns its contents.
func (p *ConfigProvider) ReadConfig(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteConfig writes the provided data to the file at the given path.
func (p *ConfigProvider) WriteConfig(path string, data []byte) error {
	// Channel configuration can carry identifiers and tokens, and Matrix is
	// single-user: there is no reader that needs group or world access.
	return os.WriteFile(path, data, 0o600)
}
