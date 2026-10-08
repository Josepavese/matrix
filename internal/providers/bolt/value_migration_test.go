package bolt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"path/filepath"
	"testing"

	bbolt "go.etcd.io/bbolt"
)

func legacyFixture(t *testing.T, text string) []byte {
	t.Helper()
	block, err := aes.NewCipher(bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := bytes.Repeat([]byte{7}, gcm.NonceSize())
	sealed := gcm.Seal(nil, nonce, []byte(text), nil)
	return []byte("ENCV1:" + base64.StdEncoding.EncodeToString(append(nonce, sealed...)))
}

func rawVault(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(defaultBucket)
		if err != nil {
			return err
		}
		for key, value := range entries {
			if err := bucket.Put([]byte(key), value); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func assertRawValue(t *testing.T, path, key string, expected []byte) {
	t.Helper()
	db, err := bbolt.Open(path, 0600, &bbolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.View(func(tx *bbolt.Tx) error {
		if !bytes.Equal(tx.Bucket(defaultBucket).Get([]byte(key)), expected) {
			t.Error("original value was changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestV1MigrationPreservesDataAndVerifiedOriginalBackup(t *testing.T) {
	setTestVaultKey(t)
	path := filepath.Join(t.TempDir(), "vault.db")
	original := legacyFixture(t, "original")
	rawVault(t, path, map[string][]byte{"bound": original, "plain": []byte("data")})
	p, err := NewProvider(path)
	if err != nil {
		t.Fatal(err)
	}
	for key, expected := range map[string]string{"bound": "original", "plain": "data"} {
		got, err := p.Get(key)
		if err != nil || string(got) != expected {
			t.Fatalf("conversion lost data: %s %v", key, err)
		}
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(path + ".pre-encv2-*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatalf("verified backup missing: %v %v", backups, err)
	}
	assertRawValue(t, backups[0], "bound", original)
	assertRawValue(t, backups[0], "plain", []byte("data"))
	p, err = NewProvider(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	again, _ := filepath.Glob(path + ".pre-encv2-*.bak")
	if len(again) != 1 {
		t.Fatal("migration ran again on already converted values")
	}
}

func TestFailedMigrationRollsBackEveryRecord(t *testing.T) {
	setTestVaultKey(t)
	path := filepath.Join(t.TempDir(), "vault.db")
	bad := []byte("ENCV1:corrupt")
	rawVault(t, path, map[string][]byte{"a_plain": []byte("preserved"), "z_bad": bad})
	if p, err := NewProvider(path); err == nil {
		_ = p.Close()
		t.Fatal("corrupt value silently migrated")
	}
	assertRawValue(t, path, "a_plain", []byte("preserved"))
	assertRawValue(t, path, "z_bad", bad)
}

func TestInsufficientMigrationSpaceRejectsBeforeBackupOrWrite(t *testing.T) {
	setTestVaultKey(t)
	path := filepath.Join(t.TempDir(), "vault.db")
	original := legacyFixture(t, "preserved")
	rawVault(t, path, map[string][]byte{"value": original})
	previous := migrationDiskSpace
	migrationDiskSpace = func(string) (diskSpace, error) { return diskSpace{}, nil }
	t.Cleanup(func() { migrationDiskSpace = previous })
	if p, err := NewProvider(path); err == nil {
		_ = p.Close()
		t.Fatal("full filesystem accepted migration")
	}
	assertRawValue(t, path, "value", original)
	backups, _ := filepath.Glob(path + ".pre-encv2-*.bak")
	if len(backups) != 0 {
		t.Fatal("disk was written before capacity check")
	}
}

func TestMigrationBackupRespectsDeclaredDirectory(t *testing.T) {
	setTestVaultKey(t)
	path := filepath.Join(t.TempDir(), "vault.db")
	dir := t.TempDir()
	t.Setenv("MATRIX_VAULT_MIGRATION_BACKUP_DIR", dir)
	original := legacyFixture(t, "preserved")
	rawVault(t, path, map[string][]byte{"value": original})
	p, err := NewProvider(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "vault.db.pre-encv2-*.bak"))
	if len(backups) != 1 {
		t.Fatal("backup did not use declared filesystem")
	}
	assertRawValue(t, backups[0], "value", original)
	local, _ := filepath.Glob(path + ".pre-encv2-*.bak")
	if len(local) != 0 {
		t.Fatal("backup also filled the source filesystem")
	}
}

func TestMovedCiphertextIsRejectedByTheStorageBoundary(t *testing.T) {
	setTestVaultKey(t)
	path := filepath.Join(t.TempDir(), "vault.db")
	p, err := NewProvider(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Set("first", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	var sealed []byte
	if err := p.db.View(func(tx *bbolt.Tx) error {
		sealed = append([]byte(nil), tx.Bucket(defaultBucket).Get([]byte("first"))...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	rawVault(t, path, map[string][]byte{"second": sealed})
	p, err = NewProvider(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	if _, err := p.Get("second"); err == nil {
		t.Fatal("storage accepted a substituted ciphertext")
	}
}
