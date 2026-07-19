package runtimevault

import (
	"github.com/Josepavese/matrix/internal/logic/matrixhome"
	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
	"github.com/Josepavese/matrix/internal/middleware"
	"github.com/Josepavese/matrix/internal/providers/bolt"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/Josepavese/matrix/internal/providers/rpcstorage"
)

type Storage interface {
	middleware.Storage
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
	provider, err := openLocal()
	if err == nil {
		return provider, nil
	}
	if broker, brokerErr := openBroker(); brokerErr == nil {
		return broker, nil
	}
	return nil, err
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
