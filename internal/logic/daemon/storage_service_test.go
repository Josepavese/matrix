package daemon

import (
	"errors"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
)

type inspectableStorage struct {
	*memstore.Storage
}

func (s *inspectableStorage) InspectRawEncryption() (int, int, error) {
	return 3, 1, nil
}

func TestStorageServiceRequiresTokenAndPreservesRawValues(t *testing.T) {
	store := memstore.New()
	service := NewStorageService(store, "runtime-secret")
	reply := runtimebroker.StorageReply{}
	if err := service.Set(&runtimebroker.StorageArgs{Key: "agent.a", Value: []byte(`{"active":true}`)}, &reply); !errors.Is(err, ErrInvalidBrokerToken) {
		t.Fatalf("expected token rejection, got %v", err)
	}
	args := &runtimebroker.StorageArgs{Key: "agent.a", Value: []byte(`{"active":true}`), Token: "runtime-secret"}
	if err := service.Set(args, &reply); err != nil {
		t.Fatal(err)
	}
	if err := service.Get(&runtimebroker.StorageArgs{Key: "agent.a", Token: "runtime-secret"}, &reply); err != nil {
		t.Fatal(err)
	}
	if string(reply.Value) != `{"active":true}` {
		t.Fatalf("raw value changed: %q", reply.Value)
	}
	if err := service.List(&runtimebroker.StorageArgs{Prefix: "agent.", Token: "runtime-secret"}, &reply); err != nil {
		t.Fatal(err)
	}
	if len(reply.Keys) != 1 || reply.Keys[0] != "agent.a" {
		t.Fatalf("unexpected keys: %v", reply.Keys)
	}
}

func TestStorageServiceReportsRawEncryptionCounts(t *testing.T) {
	service := NewStorageService(&inspectableStorage{Storage: memstore.New()}, "runtime-secret")
	reply := runtimebroker.StorageReply{}
	err := service.InspectRawEncryption(&runtimebroker.StorageArgs{Token: "runtime-secret"}, &reply)
	if err != nil {
		t.Fatal(err)
	}
	if reply.EncryptedKeys != 3 || reply.PlaintextKeys != 1 {
		t.Fatalf("unexpected encryption counts: %+v", reply)
	}
}
