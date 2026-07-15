package daemon

import (
	"errors"

	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
	"github.com/Josepavese/matrix/internal/middleware"
)

var ErrInvalidBrokerToken = errors.New("invalid runtime broker token")

type StorageService struct {
	store middleware.Storage
	token string
}

func NewStorageService(store middleware.Storage, token string) *StorageService {
	return &StorageService{store: store, token: token}
}

func (s *StorageService) Get(args *runtimebroker.StorageArgs, reply *runtimebroker.StorageReply) error {
	if err := s.authorize(args); err != nil {
		return err
	}
	value, err := s.store.Get(args.Key)
	reply.Value = value
	return err
}

func (s *StorageService) Set(args *runtimebroker.StorageArgs, _ *runtimebroker.StorageReply) error {
	if err := s.authorize(args); err != nil {
		return err
	}
	return s.store.Set(args.Key, args.Value)
}

func (s *StorageService) Delete(args *runtimebroker.StorageArgs, _ *runtimebroker.StorageReply) error {
	if err := s.authorize(args); err != nil {
		return err
	}
	return s.store.Delete(args.Key)
}

func (s *StorageService) List(args *runtimebroker.StorageArgs, reply *runtimebroker.StorageReply) error {
	if err := s.authorize(args); err != nil {
		return err
	}
	keys, err := s.store.List(args.Prefix)
	reply.Keys = keys
	return err
}

func (s *StorageService) authorize(args *runtimebroker.StorageArgs) error {
	if args == nil || s.token == "" || args.Token != s.token {
		return ErrInvalidBrokerToken
	}
	return nil
}
