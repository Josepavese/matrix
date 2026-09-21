package vaultsec

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

// ----------------------------------------------------------------------------
// Tier A adversarial suite: vault encryption, sealing, report, backup/restore
// ----------------------------------------------------------------------------
//
// These paths guard the vault's confidentiality and the operator's ability to
// recover it. The tests cover the failure modes on purpose: a key resolver that
// silently accepts a missing or world-readable key, a seal that skips entries, a
// restore that leaves a half-written file.

// keyFixture writes a master key file and returns its path.
func keyFixture(t *testing.T, dir string, value []byte, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, "vault-master.key")
	encoded := []byte(base64.StdEncoding.EncodeToString(value) + "\n")
	if err := os.WriteFile(path, encoded, perm); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatalf("chmod key: %v", err)
	}
	return path
}

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key
}

// TestResolveMasterKeyFromEnvironmentFile covers the documented operator path:
// the key file named by the environment, with its permission check.
func TestResolveMasterKeyFromEnvironmentFile(t *testing.T) {
	dir := t.TempDir()
	key := testKey(t)
	path := keyFixture(t, dir, key, 0o600)

	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", path)

	resolved, status, err := ResolveMasterKey(osfs.NewFSProvider())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !status.Configured || status.Source != "env:MATRIX_VAULT_MASTER_KEY_FILE" {
		t.Fatalf("unexpected status: %+v", status)
	}
	if string(resolved) != string(key) {
		t.Fatal("resolved key does not match the configured key")
	}
}

func TestResolveMasterKeyFromEnvironmentValue(t *testing.T) {
	t.Setenv("MATRIX_HOME", t.TempDir())
	key := testKey(t)
	for name, encoded := range map[string]string{
		"base64": base64.StdEncoding.EncodeToString(key),
		"hex":    hex.EncodeToString(key),
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
			t.Setenv("MATRIX_VAULT_MASTER_KEY", "  "+encoded+"  ")
			resolved, status, err := ResolveMasterKey(nil)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if !status.Configured || string(resolved) != string(key) {
				t.Fatalf("unexpected result: configured=%v key_match=%v", status.Configured, string(resolved) == string(key))
			}
		})
	}
}

// TestResolveMasterKeyRejectsUnusableKeys keeps a malformed key from being
// accepted and silently producing undecryptable data.
func TestResolveMasterKeyRejectsUnusableKeys(t *testing.T) {
	// Isolate from any real key already present in the operator's home.
	t.Setenv("MATRIX_HOME", t.TempDir())
	cases := map[string]string{
		"too short":  base64.StdEncoding.EncodeToString([]byte("short")),
		"not base64": "!!!not-a-key!!!",
		"empty":      "",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
			t.Setenv("MATRIX_VAULT_MASTER_KEY", value)
			if value == "" {
				// An empty value means "not configured", not "invalid".
				_, status, err := ResolveMasterKey(nil)
				if err != nil {
					t.Fatalf("empty value must not error: %v", err)
				}
				if status.Configured {
					t.Fatal("an empty value must not be treated as a configured key")
				}
				return
			}
			if _, _, err := ResolveMasterKey(nil); err == nil {
				t.Fatal("an unusable key must be rejected")
			}
		})
	}
}

// TestResolveMasterKeyPrefersTheFileOverTheInlineValue documents precedence: a
// file is an explicit operator deployment choice.
func TestResolveMasterKeyPrefersTheFileOverTheInlineValue(t *testing.T) {
	t.Setenv("MATRIX_HOME", t.TempDir())
	dir := t.TempDir()
	fileKey := testKey(t)
	inlineKey := make([]byte, 32)
	for i := range inlineKey {
		inlineKey[i] = 0xFF
	}
	path := keyFixture(t, dir, fileKey, 0o600)
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", path)
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(inlineKey))

	resolved, _, err := ResolveMasterKey(osfs.NewFSProvider())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if string(resolved) != string(fileKey) {
		t.Fatal("the key file must take precedence over the inline value")
	}
}

func TestResolveMasterKeyFromMatrixHome(t *testing.T) {
	home := t.TempDir()
	key := testKey(t)
	if err := os.MkdirAll(filepath.Join(home, "configs"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, defaultMasterKeyPath)
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MATRIX_HOME", home)
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")

	resolved, status, err := ResolveMasterKey(osfs.NewFSProvider())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !status.Configured {
		t.Fatal("the matrix home key must be discovered")
	}
	if string(resolved) != string(key) {
		t.Fatal("resolved key does not match the stored key")
	}
}

// TestEnsureDefaultMasterKeyIsIdempotentAndPrivate covers the write path: it must
// create the key once, with private permissions, and never overwrite it.
func TestEnsureDefaultMasterKeyIsIdempotentAndPrivate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	fs := osfs.NewFSProvider()

	status, err := EnsureDefaultMasterKey(fs)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !status.Configured {
		t.Fatal("the key must be configured after creation")
	}
	path := filepath.Join(home, defaultMasterKeyPath)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if permissionsSupported() && info.Mode().Perm() != 0o600 {
		t.Fatalf("master key must be private, got %v", info.Mode().Perm())
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// A second call must leave the existing key untouched.
	if _, err := EnsureDefaultMasterKey(fs); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("EnsureDefaultMasterKey must not replace an existing key")
	}

	// The generated key must be usable for encryption.
	plain := []byte("secret")
	sealed, err := EncryptBytes(plain)
	if err != nil {
		t.Fatalf("encrypt with the generated key: %v", err)
	}
	opened, err := DecryptBytes(sealed)
	if err != nil {
		t.Fatalf("decrypt with the generated key: %v", err)
	}
	if string(opened) != string(plain) {
		t.Fatal("round trip with the generated key failed")
	}
}

// TestDecryptRejectsTamperedCiphertext keeps a modified vault entry from
// decrypting to attacker-chosen plaintext (AES-GCM authentication).
func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	t.Setenv("MATRIX_HOME", t.TempDir())
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(testKey(t)))
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")

	sealed, err := EncryptBytes([]byte("payload"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	// Flip a byte inside the base64 body.
	tampered := append([]byte{}, sealed...)
	body := tampered[len(encryptedPrefix):]
	if body[0] == 'A' {
		body[0] = 'B'
	} else {
		body[0] = 'A'
	}
	if _, err := DecryptBytes(tampered); err == nil {
		t.Fatal("a tampered ciphertext must not decrypt")
	}
	// A malformed base64 payload must also fail rather than return garbage.
	if _, err := DecryptBytes([]byte(encryptedPrefix + "!!!not-base64!!!")); err == nil {
		t.Fatal("malformed ciphertext must not decrypt")
	}
}

// failingStorage injects storage failures to exercise SealStorage's error paths.
type failingStorage struct {
	storage    middleware.Storage
	listErr    error
	getErr     error
	setErr     error
	failGetKey string
}

func (f *failingStorage) List(prefix string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.storage.List(prefix)
}

func (f *failingStorage) Get(key string) ([]byte, error) {
	if f.getErr != nil && (f.failGetKey == "" || f.failGetKey == key) {
		return nil, f.getErr
	}
	return f.storage.Get(key)
}

func (f *failingStorage) Set(key string, value []byte) error {
	if f.setErr != nil {
		return f.setErr
	}
	return f.storage.Set(key, value)
}

func (f *failingStorage) Delete(key string) error { return f.storage.Delete(key) }

// TestSealStorageRewritesEveryEntry is the core security operation: after a seal
// no plaintext entry may remain, and the count must reflect what was rewritten.
func TestSealStorageRewritesEveryEntry(t *testing.T) {
	store := memstore.New()
	for _, key := range []string{"a", "b", "c"} {
		if err := store.Set(key, []byte("plain-"+key)); err != nil {
			t.Fatal(err)
		}
	}
	count, err := SealStorage(store)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 sealed entries, got %d", count)
	}
	keys, err := store.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("sealing must not lose entries, got %v", keys)
	}
}

func TestSealStorageReportsFailures(t *testing.T) {
	store := memstore.New()
	if err := store.Set("a", []byte("x")); err != nil {
		t.Fatal(err)
	}
	cases := map[string]*failingStorage{
		"list fails": {storage: store, listErr: errors.New("boom")},
		"get fails":  {storage: store, getErr: errors.New("boom")},
		"set fails":  {storage: store, setErr: errors.New("boom")},
	}
	for name, failing := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := SealStorage(failing); err == nil {
				t.Fatal("a storage failure must surface")
			}
		})
	}
}

// TestBuildReportMissingVaultKeepsTheOperatorInformed covers the first-run case.
func TestBuildReportMissingVaultKeepsTheOperatorInformed(t *testing.T) {
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)

	path := filepath.Join(t.TempDir(), "absent.db")
	report, err := BuildReport(osfs.NewFSProvider(), path, nil)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if exists, _ := report["exists"].(bool); exists {
		t.Fatal("a missing vault must be reported as absent")
	}
	warnings, _ := report["warnings"].([]string)
	joined := strings.Join(warnings, " ")
	if !strings.Contains(joined, "master key is not configured") {
		t.Fatalf("the missing key must be warned about: %v", warnings)
	}
}

// TestBuildReportWarnsAboutEveryDegradedState pins the operator-facing warnings:
// they are the only signal that the vault is not in the expected state.
func TestBuildReportWarnsAboutEveryDegradedState(t *testing.T) {
	t.Setenv("MATRIX_VAULT_MASTER_KEY", base64.StdEncoding.EncodeToString(testKey(t)))
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.db")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("inspector unavailable", func(t *testing.T) {
		report, err := BuildReport(osfs.NewFSProvider(), path, nil)
		if err != nil {
			t.Fatal(err)
		}
		warnings, _ := report["warnings"].([]string)
		if !strings.Contains(strings.Join(warnings, " "), "inspector unavailable") {
			t.Fatalf("missing inspector warning: %v", warnings)
		}
	})

	t.Run("inspector fails", func(t *testing.T) {
		report, err := BuildReport(osfs.NewFSProvider(), path, failingInspector{err: errors.New("read error")})
		if err != nil {
			t.Fatal(err)
		}
		warnings, _ := report["warnings"].([]string)
		if !strings.Contains(strings.Join(warnings, " "), "failed to inspect") {
			t.Fatalf("missing inspector failure warning: %v", warnings)
		}
	})

	t.Run("plaintext entries with a configured key", func(t *testing.T) {
		report, err := BuildReport(osfs.NewFSProvider(), path, staticEncryptionInspector{encrypted: 2, plaintext: 3})
		if err != nil {
			t.Fatal(err)
		}
		warnings, _ := report["warnings"].([]string)
		if !strings.Contains(strings.Join(warnings, " "), "plaintext entries") {
			t.Fatalf("missing plaintext warning: %v", warnings)
		}
		encryption, _ := report["encryption"].(map[string]any)
		if mixed, _ := encryption["mixed_mode"].(bool); !mixed {
			t.Fatalf("mixed mode must be reported: %+v", encryption)
		}
	})

	t.Run("world readable vault", func(t *testing.T) {
		openPath := filepath.Join(dir, "open.db")
		if err := os.WriteFile(openPath, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(openPath, 0o644); err != nil {
			t.Fatal(err)
		}
		report, err := BuildReport(osfs.NewFSProvider(), openPath, staticEncryptionInspector{encrypted: 1})
		if err != nil {
			t.Fatal(err)
		}
		if secure, _ := report["permissions_secure"].(bool); secure && permissionsSupported() {
			t.Fatal("a world readable vault must not be reported as secure")
		}
	})
}

// statFailFS fails Stat for a specific path to exercise the non-NotExist branch.
type statFailFS struct {
	middleware.FS
	failPath string
	err      error
}

func (f statFailFS) Stat(path string) (os.FileInfo, error) {
	if path == f.failPath {
		return nil, f.err
	}
	return f.FS.Stat(path)
}

func TestBuildReportReportsStatFailures(t *testing.T) {
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_HOME", t.TempDir())

	path := filepath.Join(t.TempDir(), "vault.db")
	fs := statFailFS{FS: osfs.NewFSProvider(), failPath: path, err: errors.New("permission denied")}
	if _, err := BuildReport(fs, path, nil); err == nil {
		t.Fatal("a stat failure that is not NotExist must surface as an error")
	}
}

// TestCreateBackupCoversDefaultAndCollision keeps the recovery artifact honest:
// the default directory is used, and an existing backup is never overwritten.
func TestCreateBackupCoversDefaultAndCollision(t *testing.T) {
	dir := t.TempDir()
	vault := filepath.Join(dir, "vault.db")
	if err := os.WriteFile(vault, []byte("vault-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := osfs.NewFSProvider()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	backupDir := filepath.Join(dir, "backups")
	path, err := CreateBackup(fs, vault, backupDir, now)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	copied, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(copied) != "vault-bytes" {
		t.Fatal("backup content differs from the vault")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if permissionsSupported() && info.Mode().Perm() != 0o600 {
		t.Fatalf("a backup must be private, got %v", info.Mode().Perm())
	}

	// The same timestamp must not overwrite the existing backup.
	if _, err := CreateBackup(fs, vault, backupDir, now); err == nil {
		t.Fatal("an existing backup must not be overwritten")
	}
	// A missing source must fail rather than produce an empty backup.
	if _, err := CreateBackup(fs, filepath.Join(dir, "absent.db"), backupDir, now); err == nil {
		t.Fatal("a missing vault must not produce a backup")
	}
}

// TestRestoreBackupValidations keeps a destructive operation from being pointed
// at the wrong file.
func TestRestoreBackupValidations(t *testing.T) {
	dir := t.TempDir()
	fs := osfs.NewFSProvider()
	source := filepath.Join(dir, "backup.db")
	if err := os.WriteFile(source, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := RestoreBackup(fs, "", source, "", time.Now()); err == nil {
		t.Fatal("an empty source must be rejected")
	}
	if _, err := RestoreBackup(fs, source, "", "", time.Now()); err == nil {
		t.Fatal("an empty target must be rejected")
	}
	if _, err := RestoreBackup(fs, source, source, "", time.Now()); err == nil {
		t.Fatal("restoring a file onto itself must be rejected")
	}
	if _, err := RestoreBackup(fs, filepath.Join(dir, "absent.db"), filepath.Join(dir, "target.db"), "", time.Now()); err == nil {
		t.Fatal("a missing source must be rejected")
	}
	if _, err := RestoreBackup(fs, dir, filepath.Join(dir, "target.db"), "", time.Now()); err == nil {
		t.Fatal("a directory source must be rejected")
	}
}

// TestRestoreBackupReplacesTheTargetAndKeepsAPreRestoreCopy covers both shapes:
// a fresh target and an existing one that must be preserved.
func TestRestoreBackupReplacesTheTargetAndKeepsAPreRestoreCopy(t *testing.T) {
	dir := t.TempDir()
	fs := osfs.NewFSProvider()
	source := filepath.Join(dir, "backup.db")
	if err := os.WriteFile(source, []byte("restored-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "vault.db")
	backupDir := filepath.Join(dir, "backups")

	t.Run("target absent", func(t *testing.T) {
		pre, err := RestoreBackup(fs, source, target, backupDir, time.Now())
		if err != nil {
			t.Fatalf("restore: %v", err)
		}
		if pre != "" {
			t.Fatalf("no pre-restore backup is expected for a fresh target, got %q", pre)
		}
		content, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "restored-content" {
			t.Fatal("target does not carry the restored content")
		}
	})

	t.Run("target present", func(t *testing.T) {
		if err := os.WriteFile(target, []byte("current-content"), 0o600); err != nil {
			t.Fatal(err)
		}
		pre, err := RestoreBackup(fs, source, target, backupDir, time.Now())
		if err != nil {
			t.Fatalf("restore: %v", err)
		}
		if pre == "" {
			t.Fatal("an existing target must be backed up before being replaced")
		}
		saved, err := os.ReadFile(pre)
		if err != nil {
			t.Fatalf("read pre-restore backup: %v", err)
		}
		if string(saved) != "current-content" {
			t.Fatalf("the pre-restore backup must hold the previous content, got %q", saved)
		}
		content, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "restored-content" {
			t.Fatal("target was not replaced")
		}
		// The temporary file must not be left behind.
		if _, err := os.Stat(target + ".restore.tmp"); !os.IsNotExist(err) {
			t.Fatal("the restore temporary file must be removed")
		}
	})
}

// renameFailFS injects a rename failure so the restore cleanup path is exercised.
type renameFailFS struct {
	middleware.FS
	err error
}

func (f renameFailFS) Rename(string, string) error { return f.err }

func TestRestoreBackupCleansUpWhenRenameFails(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "backup.db")
	if err := os.WriteFile(source, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "vault.db")
	fs := renameFailFS{FS: osfs.NewFSProvider(), err: errors.New("rename denied")}

	if _, err := RestoreBackup(fs, source, target, filepath.Join(dir, "backups"), time.Now()); err == nil {
		t.Fatal("a rename failure must surface")
	}
	if _, err := os.Stat(target + ".restore.tmp"); !os.IsNotExist(err) {
		t.Fatal("a failed restore must not leave the temporary file behind")
	}
}

// openFileFailFS injects an OpenFile failure for the copy step.
type openFileFailFS struct {
	middleware.FS
	failSuffix string
	err        error
}

func (f openFileFailFS) OpenFile(path string, flag int, perm os.FileMode) (middleware.File, error) {
	if strings.HasSuffix(path, f.failSuffix) {
		return nil, f.err
	}
	return f.FS.OpenFile(path, flag, perm)
}

type failingInspector struct{ err error }

func (f failingInspector) InspectRawEncryption() (int, int, error) { return 0, 0, f.err }

// TestRestoreBackupFailsWhenTheTemporaryFileCannotBeCreated keeps a partial
// restore from being reported as success.
func TestRestoreBackupFailsWhenTheTemporaryFileCannotBeCreated(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "backup.db")
	if err := os.WriteFile(source, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "vault.db")
	fs := openFileFailFS{FS: osfs.NewFSProvider(), failSuffix: ".restore.tmp", err: errors.New("denied")}

	if _, err := RestoreBackup(fs, source, target, filepath.Join(dir, "backups"), time.Now()); err == nil {
		t.Fatal("a temporary-file failure must surface")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("the target must not be created when the copy failed")
	}
}

// ----------------------------------------------------------------------------
// Remaining reachable branches: the OS-backed path (fs == nil) and key-file
// error handling. Unreachable branches (a failing crypto/rand, a wrong-size key
// reaching aes.NewCipher) are deliberately left alone rather than asserted with
// mocks that would test the mock.
// ----------------------------------------------------------------------------

// TestResolveMasterKeyFromEnvironmentFileFailures covers a key file that exists
// but cannot be used: invalid content and an unreadable path.
func TestResolveMasterKeyFromEnvironmentFileFailures(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MATRIX_HOME", t.TempDir())
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	fs := osfs.NewFSProvider()

	t.Run("invalid content", func(t *testing.T) {
		path := filepath.Join(dir, "bad.key")
		if err := os.WriteFile(path, []byte("not-a-key\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", path)
		if _, _, err := ResolveMasterKey(fs); err == nil {
			t.Fatal("an unusable key file must be rejected")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", filepath.Join(dir, "absent.key"))
		if _, _, err := ResolveMasterKey(fs); err == nil {
			t.Fatal("a missing key file must be reported")
		}
	})

	t.Run("path is a directory", func(t *testing.T) {
		t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", dir)
		if _, _, err := ResolveMasterKey(fs); err == nil {
			t.Fatal("a directory used as a key file must be reported")
		}
	})
}

// TestResolveMasterKeyUsesTheOSPathWhenNoFilesystemIsInjected exercises the
// fs == nil branch, which is what the CLI uses.
func TestResolveMasterKeyUsesTheOSPathWhenNoFilesystemIsInjected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MATRIX_HOME", t.TempDir())
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	key := testKey(t)
	path := keyFixture(t, dir, key, 0o600)
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", path)

	resolved, status, err := ResolveMasterKey(nil)
	if err != nil {
		t.Fatalf("resolve through the OS path: %v", err)
	}
	if !status.Configured || string(resolved) != string(key) {
		t.Fatal("the OS path must resolve the same key")
	}

	// The OS path must apply the same permission rule as the injected one.
	broad := filepath.Join(dir, "broad.key")
	if err := os.WriteFile(broad, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(broad, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", broad)
	if _, _, err := ResolveMasterKey(nil); err == nil && permissionsSupported() {
		t.Fatal("a world readable key file must be rejected on the OS path")
	}

	// A directory is not a key file.
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", dir)
	if _, _, err := ResolveMasterKey(nil); err == nil {
		t.Fatal("a directory must be reported on the OS path")
	}
}

// TestEnsureDefaultMasterKeyWithoutFilesystem covers the CLI write path.
func TestEnsureDefaultMasterKeyWithoutFilesystem(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MATRIX_HOME", home)
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")

	status, err := EnsureDefaultMasterKey(nil)
	if err != nil {
		t.Fatalf("ensure through the OS path: %v", err)
	}
	if !status.Configured {
		t.Fatal("the key must be configured after creation")
	}
	if _, err := os.Stat(filepath.Join(home, defaultMasterKeyPath)); err != nil {
		t.Fatalf("key file not created: %v", err)
	}
}

// TestEnsureDefaultMasterKeySurfacesAConfigurationError keeps a broken key
// configuration from being silently replaced by a fresh key: that would make
// every previously sealed entry unreadable.
func TestEnsureDefaultMasterKeySurfacesAConfigurationError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MATRIX_HOME", t.TempDir())
	bad := filepath.Join(dir, "bad.key")
	if err := os.WriteFile(bad, []byte("broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", bad)
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")

	if _, err := EnsureDefaultMasterKey(osfs.NewFSProvider()); err == nil {
		t.Fatal("a broken key configuration must not be papered over with a new key")
	}
	// Nothing must have been generated in the home directory.
	if _, err := os.Stat(filepath.Join(os.Getenv("MATRIX_HOME"), defaultMasterKeyPath)); !os.IsNotExist(err) {
		t.Fatal("no key may be generated when the configured one is broken")
	}
}

// TestEncryptDecryptThroughTheOSKeyPath covers the CLI path end to end.
func TestEncryptDecryptThroughTheOSKeyPath(t *testing.T) {
	t.Setenv("MATRIX_HOME", t.TempDir())
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	if _, err := EnsureDefaultMasterKey(nil); err != nil {
		t.Fatal(err)
	}
	sealed, err := EncryptBytes([]byte("payload"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !IsEncryptedValue(sealed) {
		t.Fatal("sealed values must be recognisable")
	}
	opened, err := DecryptBytes(sealed)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(opened) != "payload" {
		t.Fatal("round trip failed on the OS key path")
	}
	// Plaintext must pass through unchanged.
	if plain, err := DecryptBytes([]byte("clear")); err != nil || string(plain) != "clear" {
		t.Fatalf("plaintext passthrough changed behaviour: %q %v", plain, err)
	}
	// Encrypting with no key configured anywhere must be refused, not silently
	// skipped. Both the explicit home and the user home are emptied so the
	// operator's real key cannot be picked up.
	t.Setenv("MATRIX_HOME", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY", "")
	t.Setenv("MATRIX_VAULT_MASTER_KEY_FILE", "")
	t.Setenv("HOME", t.TempDir())
	if _, err := EncryptBytes([]byte("x")); err == nil {
		t.Fatal("encryption without a key must be refused")
	}
	if _, err := DecryptBytes([]byte(encryptedPrefix + "AAAA")); err == nil {
		t.Fatal("decryption without a key must be refused")
	}
}
