package bolt

import (
	"path/filepath"
	"testing"

	bbolt "go.etcd.io/bbolt"
)

func TestBackupVerifierRejectsChangedRecordsOutsideVaultBucket(t *testing.T) {
	setTestVaultKey(t)
	path := filepath.Join(t.TempDir(), "vault.db")
	rawVault(t, path, map[string][]byte{"value": legacyFixture(t, "preserved")})
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucket([]byte("other"))
		if err != nil {
			return err
		}
		return bucket.Put([]byte("extra"), []byte("preserve too"))
	}); err != nil {
		t.Fatal(err)
	}
	var expected string
	if err := db.View(func(tx *bbolt.Tx) error { var err error; expected, err = bucketFingerprint(tx); return err }); err != nil {
		t.Fatal(err)
	}
	backup, err := verifiedMigrationBackup(db, path)
	if err != nil {
		t.Fatal(err)
	}
	backupDB, err := bbolt.Open(backup, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = backupDB.Close() }()
	if err := backupDB.Update(func(tx *bbolt.Tx) error { return tx.Bucket([]byte("other")).Put([]byte("extra"), []byte("changed")) }); err != nil {
		t.Fatal(err)
	}
	if err := verifyMigrationBackup(backupDB, expected); err == nil {
		t.Fatal("backup verifier ignored data outside the default bucket")
	}
}
