package middleware

// ConfigReader abstracts the loading of configuration files from the underlying OS or storage.
// This decouples the Logic layer from platform-specific file reading according to PAL.
type ConfigReader interface {
	ReadConfig(path string) ([]byte, error)
}

// ConfigWriter abstracts the writing of configuration files to the underlying OS or storage.
type ConfigWriter interface {
	WriteConfig(path string, data []byte) error
}

// ConfigManager combines Reader and Writer for full abstraction
type ConfigManager interface {
	ConfigReader
	ConfigWriter
}
