package main

import (
	"testing"

	"github.com/Josepavese/matrix/internal/logic/config"
	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/vault"
)

func TestLocalAPIKeysAreGeneratedOnceAndPersisted(t *testing.T) {
	t.Setenv("MATRIX_LOCAL_UNAUTHENTICATED", "")
	cfg := config.NewManager(vault.NewVault(memstore.New()))
	if err := ensureLocalAPIKeys(cfg); err != nil {
		t.Fatal(err)
	}
	first := map[string]string{}
	for _, name := range []string{"matrix_api_key", "daemon_api_key"} {
		value, err := cfg.Get(name)
		if err != nil || len(value) != 64 {
			t.Fatalf("%s not generated: len=%d err=%v", name, len(value), err)
		}
		first[name] = value
	}
	if first["matrix_api_key"] == first["daemon_api_key"] {
		t.Fatal("API keys must differ")
	}
	if err := ensureLocalAPIKeys(cfg); err != nil {
		t.Fatal(err)
	}
	for name, want := range first {
		got, err := cfg.Get(name)
		if err != nil || got != want {
			t.Fatalf("%s rotated unexpectedly", name)
		}
	}
}

func TestLocalAPIKeyOptOutRequiresExplicitDevelopmentSetting(t *testing.T) {
	t.Setenv("MATRIX_LOCAL_UNAUTHENTICATED", "1")
	cfg := config.NewManager(vault.NewVault(memstore.New()))
	if err := ensureLocalAPIKeys(cfg); err != nil {
		t.Fatal(err)
	}
	if value, err := cfg.Get("matrix_api_key"); err != nil || value != "" {
		t.Fatalf("opt-out generated a key: len=%d err=%v", len(value), err)
	}
}
