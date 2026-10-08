package bolt

import (
	"fmt"
	"os"
	"path/filepath"

	bbolt "go.etcd.io/bbolt"
)

type diskSpace struct {
	free   uint64
	volume string
}

var migrationDiskSpace = availableMigrationSpace

// Reserve against live allocated pages, not the high-water file length: the
// verified compact backup preserves every record without copying free pages.
func requireMigrationSpace(db *bbolt.DB, path string) error {
	var live uint64
	if err := db.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(_ []byte, bucket *bbolt.Bucket) error {
			stats := bucket.Stats()
			live += uint64(stats.BranchAlloc + stats.LeafAlloc)
			return nil
		})
	}); err != nil {
		return err
	}
	source := filepath.Dir(path)
	backup := filepath.Dir(migrationBackupPath(path))
	if !filepath.IsAbs(backup) {
		return fmt.Errorf("migration backup directory must be absolute")
	}
	if info, err := os.Stat(backup); err != nil || !info.IsDir() {
		return fmt.Errorf("migration backup directory is unavailable")
	}
	root, err := migrationDiskSpace(source)
	if err != nil {
		return err
	}
	dest, err := migrationDiskSpace(backup)
	if err != nil {
		return err
	}
	// Copy-on-write and base64 expansion of old plaintext need additional pages.
	required := 2*live + uint64(512<<20)
	copyBudget := 2*live + uint64(512<<20)
	if root.volume == dest.volume {
		required += copyBudget
		copyBudget = 0
	}
	if root.free < required || dest.free < copyBudget {
		return fmt.Errorf("insufficient migration space: vault needs %d free bytes, backup needs %d; configure MATRIX_VAULT_MIGRATION_BACKUP_DIR on a filesystem with sufficient space", required, copyBudget)
	}
	return nil
}
