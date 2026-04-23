package logging

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/config"
)

type intSettingSpec struct {
	Key      string
	EnvKey   string
	Fallback int
	Minimum  int
	Label    string
}

type int64SettingSpec struct {
	Key      string
	EnvKey   string
	Fallback int64
	Label    string
}

type boolSettingSpec struct {
	Key      string
	EnvKey   string
	Fallback bool
	Label    string
}

type enumSettingSpec struct {
	Key      string
	EnvKey   string
	Fallback string
	Allowed  []string
}

func defaultConfig() Config {
	return Config{
		Format:     defaultFormat,
		Sink:       defaultSink,
		FilePath:   defaultFile,
		MaxBytes:   defaultMaxBytes,
		MaxBackups: defaultMaxBackups,
		StdErr:     false,
		ACPWire:    false,
	}
}

func loadCoreConfig(cfgMgr *config.Manager, cfg *Config) error {
	level, err := parseLevel(loadSetting(cfgMgr, "system.logging.level", "MATRIX_LOG_LEVEL", defaultLevel))
	if err != nil {
		return err
	}
	cfg.Level = level

	cfg.Format, err = loadEnumSetting(cfgMgr, enumSettingSpec{
		Key:      "system.logging.format",
		EnvKey:   "MATRIX_LOG_FORMAT",
		Fallback: defaultFormat,
		Allowed:  []string{"json", "text"},
	})
	if err != nil {
		return err
	}

	cfg.Sink, err = loadEnumSetting(cfgMgr, enumSettingSpec{
		Key:      "system.logging.sink",
		EnvKey:   "MATRIX_LOG_SINK",
		Fallback: defaultSink,
		Allowed:  []string{"file", "stderr", "both"},
	})
	return err
}

func loadFileConfig(cfgMgr *config.Manager, cfg *Config) error {
	var err error
	cfg.FilePath = loadSetting(cfgMgr, "system.logging.file.path", "MATRIX_LOG_FILE", defaultFile)
	cfg.MaxBytes, err = loadPositiveInt64(cfgMgr, int64SettingSpec{
		Key:      "system.logging.file.max_bytes",
		EnvKey:   "MATRIX_LOG_FILE_MAX_BYTES",
		Fallback: defaultMaxBytes,
		Label:    "logging max bytes",
	})
	if err != nil {
		return err
	}

	cfg.MaxBackups, err = loadMinimumInt(cfgMgr, intSettingSpec{
		Key:      "system.logging.file.max_backups",
		EnvKey:   "MATRIX_LOG_FILE_MAX_BACKUPS",
		Fallback: defaultMaxBackups,
		Minimum:  1,
		Label:    "logging max backups",
	})
	return err
}

func loadFlagConfig(cfgMgr *config.Manager, cfg *Config) error {
	var err error
	cfg.StdErr, err = loadBoolSetting(cfgMgr, boolSettingSpec{
		Key:      "system.logging.stderr.enabled",
		EnvKey:   "MATRIX_LOG_STDERR",
		Fallback: false,
		Label:    "logging stderr flag",
	})
	if err != nil {
		return err
	}

	cfg.ACPWire, err = loadBoolSetting(cfgMgr, boolSettingSpec{
		Key:      "system.logging.wire.acp.enabled",
		EnvKey:   "MATRIX_LOG_ACP_WIRE",
		Fallback: false,
		Label:    "ACP wire logging flag",
	})
	return err
}

func loadEnumSetting(cfgMgr *config.Manager, spec enumSettingSpec) (string, error) {
	value := strings.ToLower(loadSetting(cfgMgr, spec.Key, spec.EnvKey, spec.Fallback))
	for _, option := range spec.Allowed {
		if value == option {
			return value, nil
		}
	}
	return "", fmt.Errorf("invalid %s %q", strings.ReplaceAll(spec.Key, "system.logging.", "logging "), value)
}

func loadPositiveInt64(cfgMgr *config.Manager, spec int64SettingSpec) (int64, error) {
	value, err := parseInt64(loadSetting(cfgMgr, spec.Key, spec.EnvKey, strconv.FormatInt(spec.Fallback, 10)))
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", spec.Label, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be > 0", spec.Label)
	}
	return value, nil
}

func loadMinimumInt(cfgMgr *config.Manager, spec intSettingSpec) (int, error) {
	value, err := parseInt(loadSetting(cfgMgr, spec.Key, spec.EnvKey, strconv.Itoa(spec.Fallback)))
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", spec.Label, err)
	}
	if value < spec.Minimum {
		return 0, fmt.Errorf("%s must be >= %d", spec.Label, spec.Minimum)
	}
	return value, nil
}

func loadBoolSetting(cfgMgr *config.Manager, spec boolSettingSpec) (bool, error) {
	rawFallback := strconv.FormatBool(spec.Fallback)
	value, err := parseBool(loadSetting(cfgMgr, spec.Key, spec.EnvKey, rawFallback))
	if err != nil {
		return false, fmt.Errorf("invalid %s: %w", spec.Label, err)
	}
	return value, nil
}

func loadSetting(cfgMgr *config.Manager, key, envKey, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(envKey)); val != "" {
		return val
	}
	if cfgMgr != nil {
		if val, err := cfgMgr.Get(key); err == nil && strings.TrimSpace(val) != "" {
			return val
		}
	}
	return fallback
}
