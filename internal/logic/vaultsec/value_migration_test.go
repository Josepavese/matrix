package vaultsec

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestConverterRequiresUsableMasterKey(t *testing.T) {
	t.Setenv("MATRIX_HOME", t.TempDir())
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	for _, key := range []string{"", "invalid-key"} {
		t.Setenv("MATRIX_VAULT_MASTER_KEY", key)
		convert, err := NewValueConverter()
		if err == nil || convert != nil {
			t.Fatal("migration accepted an unavailable master key")
		}
	}
}

func TestConverterPreservesBoundValuesAndUsesOneKeySnapshot(t *testing.T) {
	t.Setenv("MATRIX_HOME", t.TempDir())
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32)))
	sealed, err := EncryptBytes("fixture.key", []byte("original"))
	if err != nil {
		t.Fatal(err)
	}
	convert, err := NewValueConverter()
	if err != nil {
		t.Fatal(err)
	}
	// A complete migration must retain its original key even if configuration
	// changes between records; the provider's final verification can then abort.
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{5}, 32)))
	preserved, err := convert("fixture.key", sealed)
	if err != nil || !bytes.Equal(preserved, sealed) {
		t.Fatalf("already-bound record changed during conversion: %v", err)
	}
	converted, err := convert("fixture.plain", []byte("plain record"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptBytes("fixture.plain", converted); err == nil {
		t.Fatal("converter used a later master key")
	}
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32)))
	plain, err := DecryptBytes("fixture.plain", converted)
	if err != nil || string(plain) != "plain record" {
		t.Fatalf("original cipher snapshot did not preserve plaintext: %v", err)
	}
}

func TestConverterRejectsCorruptedRetiredCiphertext(t *testing.T) {
	t.Setenv("MATRIX_HOME", t.TempDir())
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32)))
	convert, err := NewValueConverter()
	if err != nil {
		t.Fatal(err)
	}
	for _, corrupt := range []string{"ENCV1:invalid!", "ENCV1:" + base64.StdEncoding.EncodeToString(make([]byte, 32))} {
		if _, err := convert("fixture.key", []byte(corrupt)); err == nil {
			t.Fatal("migration silently accepted corrupted retired ciphertext")
		}
	}
}
