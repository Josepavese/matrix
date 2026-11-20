package runtimevault

import (
	"errors"
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
)

// ErrBrokerStarting means the daemon owns startup and the broker is not ready.
var ErrBrokerStarting = errors.New("daemon is starting but runtime vault broker is not ready")

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

func openWithBrokerRetry(openLocal func() (Storage, error)) (Storage, error) {
	if provider, err := openBroker(); err == nil {
		return provider, nil
	}
	if broker, starting := discoverRuntimeOwner(); broker != nil {
		return broker, nil
	} else if starting {
		return waitForStartupBroker()
	}
	provider, err := openLocal()
	if err == nil {
		return provider, nil
	}
	if broker, brokerErr := openBroker(); brokerErr == nil {
		return broker, nil
	}
	if daemonStarting() {
		return waitForStartupBroker()
	}
	return nil, err
}

func waitForStartupBroker() (Storage, error) {
	provider, err := waitForBroker(brokerStartupWait)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBrokerStarting, err)
	}
	return provider, nil
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
