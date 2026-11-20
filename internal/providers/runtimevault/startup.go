package runtimevault

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Josepavese/matrix/internal/logic/matrixhome"
	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
	"github.com/Josepavese/matrix/internal/providers/bolt"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

type daemonStorage struct {
	Storage
	close func() error
}

func (s *daemonStorage) Close() error { return s.close() }

// OpenDaemon publishes startup intent before taking the vault lock and holds
// the daemon's lease until its vault is closed. The broker retires the intent
// once its descriptor is published.
func OpenDaemon(vaultPath string) (Storage, error) {
	home, err := matrixhome.Resolve()
	if err != nil {
		return nil, err
	}
	marker := runtimebroker.StartupPath(home)
	lease, err := osfs.TryExclusiveFileLock(startupLockPath(marker))
	if err != nil {
		return nil, fmt.Errorf("claim runtime broker startup: %w", err)
	}
	if lease == nil {
		return nil, fmt.Errorf("a daemon already owns this Matrix runtime")
	}
	fs := osfs.NewFSProvider()
	if err := runtimebroker.WriteStartup(fs, marker, time.Now()); err != nil {
		return nil, errors.Join(err, lease.Close())
	}
	cleanup := func() error { return errors.Join(runtimebroker.Remove(fs, marker), lease.Close()) }
	provider, err := bolt.NewProvider(vaultPath)
	if err != nil {
		return nil, errors.Join(err, cleanup())
	}
	return &daemonStorage{Storage: provider, close: sync.OnceValue(func() error {
		return errors.Join(provider.Close(), cleanup())
	})}, nil
}

func startupLockPath(marker string) string { return marker + ".lock" }

func daemonStarting() bool {
	home, err := matrixhome.Resolve()
	if err != nil {
		return false
	}
	marker := runtimebroker.StartupPath(home)
	if _, err := osfs.NewFSProvider().Stat(marker); err != nil {
		return false
	}
	lease, err := osfs.TryExclusiveFileLock(startupLockPath(marker))
	if lease != nil {
		_ = lease.Close()
	}
	return err == nil && lease == nil
}
