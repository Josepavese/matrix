package bolt

import (
	"fmt"
	"log/slog"

	"github.com/Josepavese/matrix/internal/logic/vaultsec"
	bbolt "go.etcd.io/bbolt"
)

var (
	vaultMetadataBucket = []byte("matrix_vault_metadata")
	valueFormatKey      = []byte("value_format")
)

// migrateKeyBoundValues runs before the writer is published to any caller.
// A failed decrypt/write rolls back the whole conversion; its verified backup
// is retained and no record is removed or silently interpreted as plaintext.
func migrateKeyBoundValues(db *bbolt.DB, path string) error {
	current, keys, err := inspectValueFormat(db)
	if err != nil || current {
		return err
	}
	if len(keys) == 0 {
		return db.Update(recordValueFormat)
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
		if err := convertBucketValues(tx.Bucket(defaultBucket), keys, convert); err != nil {
			return err
		}
		return recordValueFormat(tx)
	})
	if err != nil {
		return err
	}
	slog.Info("vault values upgraded to key-bound encryption", "event", "vault_key_bound_migration", "keys", len(keys), "backup", backup)
	return nil
}

// Only an unmarked vault needs a scan. All writes through Provider.Set use
// the same current encoding, and bbolt backups preserve the format marker.
func inspectValueFormat(db *bbolt.DB) (current bool, keys []string, err error) {
	err = db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(defaultBucket)
		if meta := tx.Bucket(vaultMetadataBucket); meta != nil {
			if string(meta.Get(valueFormatKey)) != vaultsec.CurrentValueFormat || bucket == nil {
				return fmt.Errorf("invalid or unsupported vault value format metadata")
			}
			current = true
			return nil
		}
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(key, value []byte) error {
			if !vaultsec.IsEncryptedValue(value) {
				keys = append(keys, string(key))
			}
			return nil
		})
	})
	return current, keys, err
}

func recordValueFormat(tx *bbolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(defaultBucket); err != nil {
		return err
	}
	meta, err := tx.CreateBucketIfNotExists(vaultMetadataBucket)
	if err != nil {
		return err
	}
	return meta.Put(valueFormatKey, []byte(vaultsec.CurrentValueFormat))
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
