package channelcfg

import (
	"fmt"
	"path/filepath"
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
		return nil, fmt.Errorf("missing config %s", path)
	}
	return []byte(data), nil
}

func openTestConfigManager(t *testing.T) (*bolt.Provider, *config.Manager) {
	t.Helper()

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
