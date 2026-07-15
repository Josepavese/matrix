package daemon

import (
	"errors"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
)

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
