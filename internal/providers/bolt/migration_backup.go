package bolt

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/vaultsec"
	bbolt "go.etcd.io/bbolt"
)

func verifiedMigrationBackup(db *bbolt.DB, path string) (string, error) {
	backup := migrationBackupPath(path)
	file, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	backupDB, err := bbolt.Open(backup, 0600, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = backupDB.Close() }()
	var expected string
	err = db.View(func(tx *bbolt.Tx) error { var err error; expected, err = bucketFingerprint(tx); return err })
	if err != nil {
		return "", err
	}
	if err := bbolt.Compact(backupDB, db, 64<<20); err != nil {
		return "", err
	}
	if err := backupDB.Sync(); err != nil {
		return "", err
	}
	if err := vaultsec.ApplySecurePermissions(backup); err != nil {
		return "", err
	}
	return backup, verifyMigrationBackup(backupDB, expected)
}

func migrationBackupPath(path string) string {
	dir := strings.TrimSpace(os.Getenv("MATRIX_VAULT_MIGRATION_BACKUP_DIR"))
	if dir == "" {
		dir, _ = filepath.Abs(filepath.Dir(path))
	}
	return filepath.Join(dir, fmt.Sprintf("%s.pre-encv2-%d.bak", filepath.Base(path), time.Now().UTC().UnixNano()))
}

func verifyMigrationBackup(db *bbolt.DB, expected string) error {
	return db.View(func(tx *bbolt.Tx) error {
		var invalid error
		for err := range tx.Check() {
			if invalid == nil {
				invalid = err
			}
		}
		if invalid != nil {
			return invalid
		}
		actual, err := bucketFingerprint(tx)
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("pre-migration backup differs from the original records")
		}
		return nil
	})
}

func bucketFingerprint(tx *bbolt.Tx) (string, error) {
	h := sha256.New()
	err := tx.ForEach(func(name []byte, bucket *bbolt.Bucket) error {
		_, _ = fmt.Fprintf(h, "%d:", len(name))
		_, _ = h.Write(name)
		return fingerprintBucket(h, bucket)
	})
	return fmt.Sprintf("%x", h.Sum(nil)), err
}

func fingerprintBucket(h hash.Hash, bucket *bbolt.Bucket) error {
	err := bucket.ForEach(func(key, value []byte) error {
		_, _ = fmt.Fprintf(h, "%d:", len(key))
		_, _ = h.Write(key)
		if child := bucket.Bucket(key); child != nil {
			_, _ = h.Write([]byte("["))
			return fingerprintBucket(h, child)
		}
		_, _ = fmt.Fprintf(h, "%d:", len(value))
		_, _ = h.Write(value)
		return nil
	})
	_, _ = h.Write([]byte("]"))
	return err
}
