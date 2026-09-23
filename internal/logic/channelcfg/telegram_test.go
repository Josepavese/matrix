package channelcfg

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/config"
	"github.com/Josepavese/matrix/internal/logic/vault"
	"github.com/Josepavese/matrix/internal/providers/bolt"
)

func TestLoadTelegramConfigUsesChannelNamespace(t *testing.T) {
	provider, cfgMgr := openTestConfigManager(t)

	if err := cfgMgr.Set(telegramTokenKey, "vault-token"); err != nil {
		t.Fatalf("set token: %v", err)
	}
	if err := cfgMgr.Set(telegramEnabledKey, "false"); err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	if err := cfgMgr.Set(telegramAdminsKey, "[]"); err != nil {
		t.Fatalf("set admins: %v", err)
	}

	cfg, source, err := LoadTelegramConfig(newTestConfigReader(map[string]string{
		"configs/telegram.json": seedTelegramConfig,
	}), cfgMgr)
	_ = provider.Close()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Token != "vault-token" {
		t.Fatalf("expected token override, got %q", cfg.Token)
	}
	if cfg.Enabled {
		t.Fatalf("expected enabled=false override to be applied")
	}
	if len(cfg.Admins) != 0 {
		t.Fatalf("expected admins to be cleared, got %v", cfg.Admins)
	}
	if source != "vault(channel.telegram.*)+seed(configs/telegram.json)" {
		t.Fatalf("unexpected source %q", source)
	}
}

func TestLoadTelegramConfigFileOverrideAndEnvOverride(t *testing.T) {
	t.Setenv("MATRIX_TELEGRAM_CONFIG", "testdata/telegram-override.json")
	t.Setenv("MATRIX_TELEGRAM_TOKEN", "env-token")
	t.Setenv("MATRIX_TELEGRAM_ENABLED", "false")
	t.Setenv("MATRIX_TELEGRAM_ADMINS", "")

	provider, cfgMgr := openTestConfigManager(t)
	if err := cfgMgr.Set(telegramTokenKey, "vault-token"); err != nil {
		t.Fatalf("set token: %v", err)
	}

	cfg, source, err := LoadTelegramConfig(newTestConfigReader(map[string]string{
		"configs/telegram.json":           seedTelegramConfig,
		"testdata/telegram-override.json": telegramOverrideConfig,
	}), cfgMgr)
	_ = provider.Close()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Token != "env-token" {
		t.Fatalf("expected env token override, got %q", cfg.Token)
	}
	if cfg.Enabled {
		t.Fatalf("expected env enabled=false override")
	}
	if len(cfg.Admins) != 0 {
		t.Fatalf("expected admins cleared by env override, got %v", cfg.Admins)
	}
	if source != "env+vault(channel.telegram.*)+file(testdata/telegram-override.json)+seed(configs/telegram.json)" {
		t.Fatalf("unexpected source %q", source)
	}
}

const seedTelegramConfig = `{
  "token": "",
  "enabled": false,
  "admins": [123, 456]
}`

const telegramOverrideConfig = `{
  "token": "file-token",
  "enabled": true,
  "admins": [999]
}`

type testConfigReader struct {
	files map[string]string
}

func newTestConfigReader(files map[string]string) testConfigReader {
	return testConfigReader{files: files}
}

func (r testConfigReader) ReadConfig(path string) ([]byte, error) {
	data, ok := r.files[path]
	if !ok {
		// Faithful to the real reader, which returns an *os.PathError: a stub that
		// returned a plain error would let the absent-file path pass here and fail in
		// production.
		return nil, fmt.Errorf("missing config %s: %w", path, fs.ErrNotExist)
	}
	return []byte(data), nil
}

func openTestConfigManager(t *testing.T) (*bolt.Provider, *config.Manager) {
	t.Helper()
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{6}, 32)))

	dbPath := filepath.Join(t.TempDir(), "matrix-vault.db")
	provider, err := bolt.NewProvider(dbPath)
	if err != nil {
		t.Fatalf("open provider: %v", err)
	}
	t.Cleanup(func() {
		_ = provider.Close()
	})

	return provider, config.NewManager(vault.NewVault(provider))
}

// TestAnAbsentTelegramSeedIsUnconfiguredNotBroken: a fresh PAL home has no
// configs/telegram.json, and the runtime used to report that absence as a channel
// failure on every start. Absence means the channel was never configured; a file that
// exists but cannot be used stays an error.
func TestAnAbsentTelegramSeedIsUnconfiguredNotBroken(t *testing.T) {
	reader := newTestConfigReader(map[string]string{})

	cfg, source, err := loadTelegramSeed(reader)
	if err != nil {
		t.Fatalf("an absent seed must not be an error: %v", err)
	}
	if cfg.Enabled || cfg.Token != "" {
		t.Fatalf("an absent seed must leave the channel unconfigured: %+v", cfg)
	}
	if !strings.Contains(source, "absent") {
		t.Fatalf("the source should say the file is absent, got %q", source)
	}

	// The errors that mean something must survive.
	malformed := newTestConfigReader(map[string]string{"configs/telegram.json": "{not json"})
	if _, _, err := loadTelegramSeed(malformed); err == nil {
		t.Fatal("a malformed seed must still be an error")
	}
	liveToken := newTestConfigReader(map[string]string{
		"configs/telegram.json": `{"enabled":true,"token":"123:live-token"}`,
	})
	if _, _, err := loadTelegramSeed(liveToken); err == nil {
		t.Fatal("a seed carrying a live token must still be an error")
	}
}
