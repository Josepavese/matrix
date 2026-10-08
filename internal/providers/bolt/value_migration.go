package bolt

import (
	"fmt"
	"log/slog"

	"github.com/Josepavese/matrix/internal/logic/vaultsec"
	bbolt "go.etcd.io/bbolt"
)

// migrateKeyBoundValues runs before the writer is published to any caller.
// A failed decrypt/write rolls back the whole conversion; its verified backup
// is retained and no record is removed or silently interpreted as plaintext.
func migrateKeyBoundValues(db *bbolt.DB, path string) error {
	var keys []string
	err := db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(defaultBucket).ForEach(func(key, value []byte) error {
			if !vaultsec.IsEncryptedValue(value) {
				keys = append(keys, string(key))
			}
			return nil
		})
	})
	if err != nil || len(keys) == 0 {
		return err
	}
	convert, err := vaultsec.NewValueConverter()
	if err != nil {
		return err
	}
	if err := requireMigrationSpace(db, path); err != nil {
		return err
	}
	backup, err := verifiedMigrationBackup(db, path)
	if err != nil {
		return err
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		return convertBucketValues(tx.Bucket(defaultBucket), keys, convert)
	})
	if err != nil {
		return err
	}
	slog.Info("vault values upgraded to key-bound encryption", "event", "vault_key_bound_migration", "keys", len(keys), "backup", backup)
	return nil
}

func convertBucketValues(bucket *bbolt.Bucket, keys []string, convert func(string, []byte) ([]byte, error)) error {
	for _, key := range keys {
		converted, err := convert(key, bucket.Get([]byte(key)))
		if err != nil {
			return fmt.Errorf("convert key %s: %w", key, err)
		}
		if err := bucket.Put([]byte(key), converted); err != nil {
			return err
		}
		if _, err := vaultsec.DecryptBytes(key, bucket.Get([]byte(key))); err != nil {
			return err
		}
	}
	return nil
}
