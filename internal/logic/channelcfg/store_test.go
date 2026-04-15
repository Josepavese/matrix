package channelcfg

import "testing"

func TestProviderKeyRejectsUnsupportedProvider(t *testing.T) {
	if _, err := ProviderKey("discord", "token"); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

func TestProviderKeyBuildsTelegramNamespace(t *testing.T) {
	key, err := ProviderKey("telegram", "token")
	if err != nil {
		t.Fatalf("provider key: %v", err)
	}
	if key != "channel.telegram.token" {
		t.Fatalf("unexpected key %q", key)
	}
}

func TestProviderKeyRejectsUnsupportedKey(t *testing.T) {
	if _, err := ProviderKey("telegram", "probe"); err == nil {
		t.Fatal("expected unsupported key error")
	}
}
