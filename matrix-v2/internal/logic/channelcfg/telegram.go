package channelcfg

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jose/matrix-v2/internal/logic/config"
	"github.com/jose/matrix-v2/internal/middleware"
)

// TelegramConfig holds the resolved Telegram channel configuration.
type TelegramConfig struct {
	Token   string  `json:"token"`
	Enabled bool    `json:"enabled"`
	Admins  []int64 `json:"admins"`
}

type telegramFileConfig struct {
	Token   string  `json:"token"`
	Enabled bool    `json:"enabled"`
	Admins  []int64 `json:"admins"`
}

type telegramFileOverlay struct {
	Token   *string  `json:"token"`
	Enabled *bool    `json:"enabled"`
	Admins  *[]int64 `json:"admins"`
}

const (
	telegramTokenKey   = "channel.telegram.token"
	telegramEnabledKey = "channel.telegram.enabled"
	telegramAdminsKey  = "channel.telegram.admins"
)

type telegramKeySet struct {
	Token   string
	Enabled string
	Admins  string
	Source  string
}

type telegramOverlay struct {
	Token      string
	HasToken   bool
	Enabled    bool
	HasEnabled bool
	Admins     []int64
	HasAdmins  bool
}

// LoadTelegramConfig loads Telegram configuration from seed, file override, and SSOT overrides.
func LoadTelegramConfig(reader middleware.ConfigReader, cfgMgr *config.Manager) (TelegramConfig, string, error) {
	cfg, source, err := loadTelegramSeed(reader)
	if err != nil {
		return TelegramConfig{}, "", err
	}
	if overridePath := strings.TrimSpace(os.Getenv("MATRIX_TELEGRAM_CONFIG")); overridePath != "" {
		override, overrideSource, err := loadTelegramFileOverride(reader, overridePath)
		if err != nil {
			return TelegramConfig{}, source, err
		}
		cfg = mergeTelegramConfig(cfg, override)
		source = overrideSource + "+" + source
	}

	vaultOverlay, vaultSource, vaultUsed, err := loadTelegramVaultOverrides(cfgMgr)
	if err != nil {
		return TelegramConfig{}, source, err
	}
	if vaultUsed {
		cfg = mergeTelegramConfig(cfg, vaultOverlay)
		source = vaultSource + "+" + source
	}

	envOverlay, envUsed, err := loadTelegramEnvOverrides()
	if err != nil {
		return TelegramConfig{}, source, err
	}
	if envUsed {
		cfg = mergeTelegramConfig(cfg, envOverlay)
		source = "env+" + source
	}

	return cfg, source, nil
}

func loadTelegramSeed(reader middleware.ConfigReader) (TelegramConfig, string, error) {
	data, err := reader.ReadConfig("configs/telegram.json")
	if err != nil {
		return TelegramConfig{}, "", fmt.Errorf("failed to read telegram seed config: %w", err)
	}
	var cfg telegramFileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return TelegramConfig{}, "configs/telegram.json", fmt.Errorf("invalid telegram seed config in configs/telegram.json: %w", err)
	}
	if strings.TrimSpace(cfg.Token) != "" {
		return TelegramConfig{}, "configs/telegram.json", fmt.Errorf("configs/telegram.json must not contain a live telegram token")
	}
	return TelegramConfig{
		Token:   "",
		Enabled: cfg.Enabled,
		Admins:  append([]int64{}, cfg.Admins...),
	}, "seed(configs/telegram.json)", nil
}

func loadTelegramFileOverride(reader middleware.ConfigReader, path string) (telegramOverlay, string, error) {
	data, err := reader.ReadConfig(path)
	if err != nil {
		return telegramOverlay{}, path, fmt.Errorf("failed to read telegram override config %s: %w", path, err)
	}
	var override telegramFileOverlay
	if err := json.Unmarshal(data, &override); err != nil {
		return telegramOverlay{}, path, fmt.Errorf("invalid telegram override config in %s: %w", path, err)
	}
	return telegramOverlayFromFile(override), "file(" + path + ")", nil
}

func loadTelegramVaultOverrides(cfgMgr *config.Manager) (telegramOverlay, string, bool, error) {
	if cfgMgr == nil {
		return telegramOverlay{}, "", false, nil
	}

	keys, err := cfgMgr.List()
	if err != nil {
		return telegramOverlay{}, "", false, err
	}
	existing := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		existing[key] = struct{}{}
	}

	channelKeys := telegramKeySet{
		Token:   telegramTokenKey,
		Enabled: telegramEnabledKey,
		Admins:  telegramAdminsKey,
		Source:  "vault(channel.telegram.*)",
	}
	channelOverlay, channelUsed, err := loadTelegramOverlayForKeys(cfgMgr, existing, channelKeys)
	if err != nil {
		return telegramOverlay{}, "", false, err
	}
	if channelUsed {
		return channelOverlay, channelKeys.Source, true, nil
	}

	return telegramOverlay{}, "", false, nil
}

func loadTelegramOverlayForKeys(cfgMgr *config.Manager, existing map[string]struct{}, keys telegramKeySet) (telegramOverlay, bool, error) {
	var overlay telegramOverlay
	used := false

	if hasKey(existing, keys.Token) {
		token, err := cfgMgr.Get(keys.Token)
		if err != nil {
			return telegramOverlay{}, false, err
		}
		overlay.Token = strings.TrimSpace(token)
		overlay.HasToken = true
		used = true
	}

	if hasKey(existing, keys.Enabled) {
		rawEnabled, err := cfgMgr.Get(keys.Enabled)
		if err != nil {
			return telegramOverlay{}, false, err
		}
		enabled, parseErr := strconv.ParseBool(strings.TrimSpace(rawEnabled))
		if parseErr != nil {
			return telegramOverlay{}, false, fmt.Errorf("invalid vault value for %s: %w", keys.Enabled, parseErr)
		}
		overlay.Enabled = enabled
		overlay.HasEnabled = true
		used = true
	}

	if hasKey(existing, keys.Admins) {
		rawAdmins, err := cfgMgr.Get(keys.Admins)
		if err != nil {
			return telegramOverlay{}, false, err
		}
		admins, parseErr := parseTelegramAdmins(rawAdmins)
		if parseErr != nil {
			return telegramOverlay{}, false, fmt.Errorf("invalid vault value for %s: %w", keys.Admins, parseErr)
		}
		overlay.Admins = admins
		overlay.HasAdmins = true
		used = true
	}

	return overlay, used, nil
}

func loadTelegramEnvOverrides() (telegramOverlay, bool, error) {
	var overlay telegramOverlay
	used := false

	if token, ok := os.LookupEnv("MATRIX_TELEGRAM_TOKEN"); ok {
		overlay.Token = strings.TrimSpace(token)
		overlay.HasToken = true
		used = true
	}
	if rawEnabled, ok := os.LookupEnv("MATRIX_TELEGRAM_ENABLED"); ok {
		enabled, err := strconv.ParseBool(strings.TrimSpace(rawEnabled))
		if err != nil {
			return telegramOverlay{}, false, fmt.Errorf("invalid MATRIX_TELEGRAM_ENABLED value %q: %w", rawEnabled, err)
		}
		overlay.Enabled = enabled
		overlay.HasEnabled = true
		used = true
	}
	if rawAdmins, ok := os.LookupEnv("MATRIX_TELEGRAM_ADMINS"); ok {
		admins, err := parseTelegramAdmins(rawAdmins)
		if err != nil {
			return telegramOverlay{}, false, fmt.Errorf("invalid MATRIX_TELEGRAM_ADMINS value %q: %w", rawAdmins, err)
		}
		overlay.Admins = admins
		overlay.HasAdmins = true
		used = true
	}

	return overlay, used, nil
}

func mergeTelegramConfig(base TelegramConfig, overlay telegramOverlay) TelegramConfig {
	if overlay.HasToken {
		base.Token = overlay.Token
	}
	if overlay.HasEnabled {
		base.Enabled = overlay.Enabled
	}
	if overlay.HasAdmins {
		base.Admins = overlay.Admins
	}
	return base
}

func telegramOverlayFromFile(fileCfg telegramFileOverlay) telegramOverlay {
	var overlay telegramOverlay
	if fileCfg.Token != nil {
		overlay.Token = strings.TrimSpace(*fileCfg.Token)
		overlay.HasToken = true
	}
	if fileCfg.Enabled != nil {
		overlay.Enabled = *fileCfg.Enabled
		overlay.HasEnabled = true
	}
	if fileCfg.Admins != nil {
		overlay.Admins = append([]int64{}, (*fileCfg.Admins)...)
		overlay.HasAdmins = true
	}
	return overlay
}

func parseTelegramAdmins(raw string) ([]int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []int64{}, nil
	}

	if strings.HasPrefix(raw, "[") {
		var admins []int64
		if err := json.Unmarshal([]byte(raw), &admins); err != nil {
			return nil, err
		}
		return admins, nil
	}

	parts := strings.Split(raw, ",")
	admins := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		adminID, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, err
		}
		admins = append(admins, adminID)
	}
	return admins, nil
}

func hasKey(existing map[string]struct{}, key string) bool {
	_, ok := existing[key]
	return ok
}
