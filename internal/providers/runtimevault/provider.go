package runtimevault

import (
	"fmt"
	"time"

	"github.com/Josepavese/matrix/internal/logic/matrixhome"
	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/bolt"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/Josepavese/matrix/internal/providers/rpcstorage"
)

const (
	brokerDiscoveryGrace = 50 * time.Millisecond
	brokerStartupWait    = 2 * time.Second
	startupMarkerMaxAge  = 5 * time.Second
)

type Storage interface {
	middleware.Storage
	middleware.RawEncryptionInspector
	Close() error
}

func Open(vaultPath string) (Storage, error) {
	return openWithBrokerRetry(func() (Storage, error) {
		return bolt.NewProvider(vaultPath)
	})
}

func OpenReadOnly(vaultPath string) (Storage, error) {
	return openWithBrokerRetry(func() (Storage, error) {
		return bolt.NewReadOnlyProvider(vaultPath)
	})
}

// OpenDaemon claims startup ownership before taking the daemon's bbolt lock.
func OpenDaemon(vaultPath string) (Storage, error) {
	home, err := matrixhome.Resolve()
	if err != nil {
		return nil, err
	}
	fs := osfs.NewFSProvider()
	marker := runtimebroker.StartupPath(home)
	if err := runtimebroker.ClaimStartup(fs, marker, time.Now(), startupMarkerMaxAge); err != nil {
		return nil, err
	}
	provider, openErr := bolt.NewProvider(vaultPath)
	removeErr := runtimebroker.RemoveStartup(fs, marker)
	if openErr != nil {
		return nil, openErr
	}
	if removeErr != nil {
		_ = provider.Close()
		return nil, fmt.Errorf("remove runtime broker startup marker: %w", removeErr)
	}
	return provider, nil
}

func openWithBrokerRetry(openLocal func() (Storage, error)) (Storage, error) {
	if provider, err := openBroker(); err == nil {
		return provider, nil
	}
	if broker, starting := discoverRuntimeOwner(); broker != nil {
		return broker, nil
	} else if starting {
		if provider, err := waitForBroker(brokerStartupWait); err == nil {
			return provider, nil
		} else {
			return nil, fmt.Errorf("daemon is starting but runtime vault broker is not ready: %w", err)
		}
	}
	provider, err := openLocal()
	if err == nil {
		return provider, nil
	}
	if broker, brokerErr := openBroker(); brokerErr == nil {
		return broker, nil
	}
	return nil, err
}

func discoverRuntimeOwner() (*rpcstorage.Provider, bool) {
	deadline := time.Now().Add(brokerDiscoveryGrace)
	for {
		if provider, err := openBroker(); err == nil {
			return provider, false
		}
		if daemonStarting() {
			return nil, true
		}
		if time.Now().After(deadline) {
			return nil, false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForBroker(timeout time.Duration) (*rpcstorage.Provider, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if provider, err := openBroker(); err == nil {
			return provider, nil
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, lastErr
}

func daemonStarting() bool {
	home, err := matrixhome.Resolve()
	if err != nil {
		return false
	}
	fresh, err := runtimebroker.StartupFresh(
		osfs.NewFSProvider(), runtimebroker.StartupPath(home), time.Now(), startupMarkerMaxAge,
	)
	return err == nil && fresh
}

func RuntimeLogFile() (string, bool) {
	descriptor, err := readDescriptor()
	return descriptor.LogFile, err == nil && descriptor.LogFile != ""
}

func openBroker() (*rpcstorage.Provider, error) {
	descriptor, err := readDescriptor()
	if err != nil {
		return nil, err
	}
	return rpcstorage.New(descriptor.JSONRPCAddr, descriptor.Token)
}

func readDescriptor() (runtimebroker.Descriptor, error) {
	home, err := matrixhome.Resolve()
	if err != nil {
		return runtimebroker.Descriptor{}, err
	}
	descriptor, err := runtimebroker.Read(osfs.NewFSProvider(), runtimebroker.Path(home))
	return descriptor, err
}
