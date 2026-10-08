package vaultsec

import (
	"testing"
)

func TestEncryptedValueCannotBeMovedToAnotherKey(t *testing.T) {
	t.Setenv("MATRIX_HOME", t.TempDir())
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	if _, err := EnsureDefaultMasterKey(nil); err != nil {
		t.Fatal(err)
	}
	sealed, err := EncryptBytes("config.a", []byte("sensitive"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptBytes("config.b", sealed); err == nil {
		t.Fatal("ciphertext authenticated under a different key")
	}
	if _, err := DecryptBytes("config.a", sealed); err != nil {
		t.Fatal(err)
	}
	convert, err := NewValueConverter()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := convert("config.b", sealed); err == nil {
		t.Fatal("migration accepted an already-bound substituted ciphertext")
	}
}

func TestMigrationRejectsUnknownEncryptedFormatAndEmptyKey(t *testing.T) {
	t.Setenv("MATRIX_HOME", t.TempDir())
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	if _, err := EnsureDefaultMasterKey(nil); err != nil {
		t.Fatal(err)
	}
	convert, err := NewValueConverter()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := convert("config.a", []byte("ENCV3:unknown")); err == nil {
		t.Fatal("unknown encryption was treated as plaintext")
	}
	if _, err := EncryptBytes("", []byte("value")); err == nil {
		t.Fatal("encrypted without storage identity")
	}
	if _, err := DecryptBytes("config.a", []byte("plaintext")); err == nil {
		t.Fatal("live reader bypassed migration")
	}
}
