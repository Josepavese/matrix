package channelruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/config"
	"github.com/Josepavese/matrix/internal/logic/vault"
	"github.com/Josepavese/matrix/internal/middleware"
)

type fakeGateway struct {
	started bool
	stopped bool
}

func (g *fakeGateway) Start(context.Context) error { g.started = true; return nil }
func (g *fakeGateway) Stop() error                 { g.stopped = true; return nil }

type fakeFactory struct {
	name    string
	gateway *fakeGateway
	enabled bool
	err     error
}

func (f fakeFactory) Name() string { return f.name }
func (f fakeFactory) Build(middleware.ConfigReader, *config.Manager, middleware.SessionRouter, Deps) (middleware.MessagingGateway, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	if !f.enabled {
		return nil, false, nil
	}
	return f.gateway, true, nil
}

type memStorage struct{ values map[string][]byte }

func (m memStorage) Get(key string) ([]byte, error)   { return m.values[key], nil }
func (m memStorage) Set(key string, val []byte) error { m.values[key] = val; return nil }
func (m memStorage) Delete(key string) error          { delete(m.values, key); return nil }
func (m memStorage) List(_ string) ([]string, error) {
	return nil, nil
}

type fakeReader struct{}

func (fakeReader) ReadConfig(string) ([]byte, error) { return nil, nil }

type fakeRouter struct{}

func (fakeRouter) Route(context.Context, string, string, string, middleware.ThoughtNotifier) (string, error) {
	return "", nil
}

func (fakeRouter) HandleSessionAction(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (fakeRouter) HandleSessionActionTyped(context.Context, middleware.SessionActionRequest) (middleware.SessionActionResult, error) {
	return middleware.SessionActionResult{}, nil
}

func (fakeRouter) HandleWorkspaceAction(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (fakeRouter) HandleWorkspaceActionTyped(context.Context, middleware.WorkspaceActionRequest) (middleware.WorkspaceActionResult, error) {
	return middleware.WorkspaceActionResult{}, nil
}

func (fakeRouter) HandleWorkspaceRead(context.Context, string, string, string, int) (string, error) {
	return "", nil
}

func (fakeRouter) HandleWorkspaceReadTyped(context.Context, middleware.WorkspaceReadRequest) (middleware.WorkspaceReadResult, error) {
	return middleware.WorkspaceReadResult{}, nil
}

func (fakeRouter) HandleIntent(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (fakeRouter) HandleIntentTyped(context.Context, middleware.IntentActionRequest) (middleware.IntentActionResult, error) {
	return middleware.IntentActionResult{}, nil
}

func TestStartAllAndStopAll(t *testing.T) {
	cfgMgr := config.NewManager(vault.NewVault(memStorage{values: map[string][]byte{}}))
	gateway := &fakeGateway{}

	started, err := StartAll(context.Background(), fakeReader{}, cfgMgr, fakeRouter{}, Deps{}, fakeFactory{name: "fake", gateway: gateway, enabled: true})
	if err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}
	if len(started) != 1 || !gateway.started {
		t.Fatalf("expected started gateway")
	}
	if err := StopAll(started); err != nil {
		t.Fatalf("StopAll failed: %v", err)
	}
	if !gateway.stopped {
		t.Fatalf("expected stopped gateway")
	}
}

func TestStartAll_PropagatesFactoryError(t *testing.T) {
	cfgMgr := config.NewManager(vault.NewVault(memStorage{values: map[string][]byte{}}))
	_, err := StartAll(context.Background(), fakeReader{}, cfgMgr, fakeRouter{}, Deps{}, fakeFactory{name: "broken", err: errors.New("boom")})
	if err == nil {
		t.Fatal("expected error")
	}
}

// staticReader serves the given config files by path.
type staticReader map[string][]byte

func (r staticReader) ReadConfig(path string) ([]byte, error) {
	if data, ok := r[path]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("config %q not found", path)
}

const enabledTelegramSeed = `{"token":"","enabled":true,"admins":[]}`

// TestTelegramFactoryRefusesEnabledChannelWithEmptyAdmins is the fail-closed
// boundary: an enabled channel without admins must not start, and the error
// must name the missing key instead of leaving an open bot running.
func TestTelegramFactoryRefusesEnabledChannelWithEmptyAdmins(t *testing.T) {
	t.Setenv("MATRIX_TELEGRAM_CONFIG", "")
	t.Setenv("MATRIX_TELEGRAM_ENABLED", "true")
	t.Setenv("MATRIX_TELEGRAM_TOKEN", "test-token")
	t.Setenv("MATRIX_TELEGRAM_ADMINS", "")

	cfgMgr := config.NewManager(vault.NewVault(memStorage{values: map[string][]byte{}}))
	gateway, enabled, err := telegramFactory{}.Build(
		staticReader{"configs/telegram.json": []byte(enabledTelegramSeed)}, cfgMgr, fakeRouter{}, Deps{})
	if err == nil {
		t.Fatal("enabled telegram channel with empty admins must be refused")
	}
	if gateway != nil || enabled {
		t.Fatalf("refused channel must not start: gateway=%v enabled=%v", gateway, enabled)
	}
	if !strings.Contains(err.Error(), "channel.telegram.admins") {
		t.Fatalf("refusal must name the empty config key, got %q", err)
	}
}

// TestTelegramFactorySkipsDisabledChannelWithEmptyAdmins keeps the refusal
// scoped to startable channels: a disabled channel stays a no-op.
func TestTelegramFactorySkipsDisabledChannelWithEmptyAdmins(t *testing.T) {
	t.Setenv("MATRIX_TELEGRAM_CONFIG", "")
	t.Setenv("MATRIX_TELEGRAM_ENABLED", "false")
	t.Setenv("MATRIX_TELEGRAM_TOKEN", "test-token")
	t.Setenv("MATRIX_TELEGRAM_ADMINS", "")

	cfgMgr := config.NewManager(vault.NewVault(memStorage{values: map[string][]byte{}}))
	gateway, enabled, err := telegramFactory{}.Build(
		staticReader{"configs/telegram.json": []byte(enabledTelegramSeed)}, cfgMgr, fakeRouter{}, Deps{})
	if err != nil {
		t.Fatalf("disabled channel must not fail startup: %v", err)
	}
	if gateway != nil || enabled {
		t.Fatalf("disabled channel must not start: gateway=%v enabled=%v", gateway, enabled)
	}
}
