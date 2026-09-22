package rpcstorage

import (
	"errors"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtimebroker"
)

// broker is a stand-in for the runtime vault broker. It records the token it
// received on every call, because a client that stops sending the token must
// fail loudly here rather than quietly in production.
type broker struct {
	mu     sync.Mutex
	tokens []string
	data   map[string][]byte
	fail   string
}

func (b *broker) record(token string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokens = append(b.tokens, token)
	if b.fail != "" {
		return errors.New(b.fail)
	}
	return nil
}

func (b *broker) Get(args runtimebroker.StorageArgs, reply *runtimebroker.StorageReply) error {
	if err := b.record(args.Token); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	reply.Value = b.data[args.Key]
	return nil
}

func (b *broker) Set(args runtimebroker.StorageArgs, reply *runtimebroker.StorageReply) error {
	if err := b.record(args.Token); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	stored := make([]byte, len(args.Value))
	copy(stored, args.Value)
	b.data[args.Key] = stored
	return nil
}

func (b *broker) Delete(args runtimebroker.StorageArgs, reply *runtimebroker.StorageReply) error {
	if err := b.record(args.Token); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.data, args.Key)
	return nil
}

func (b *broker) List(args runtimebroker.StorageArgs, reply *runtimebroker.StorageReply) error {
	if err := b.record(args.Token); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for key := range b.data {
		if strings.HasPrefix(key, args.Prefix) {
			reply.Keys = append(reply.Keys, key)
		}
	}
	return nil
}

// startBroker serves the JSON-RPC codec the provider speaks and returns the
// address to dial.
func startBroker(t *testing.T, impl *broker) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := rpc.NewServer()
	if err := server.RegisterName("Storage", impl); err != nil {
		_ = listener.Close()
		t.Fatalf("register: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go server.ServeCodec(jsonrpc.NewServerCodec(conn))
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("broker did not stop")
		}
	})
	return listener.Addr().String()
}

// TestProviderRoundTripAndTokenPropagation is the security-relevant case: every
// call must carry the operator token, since the broker trusts it to authorise
// vault access.
func TestProviderRoundTripAndTokenPropagation(t *testing.T) {
	impl := &broker{data: map[string][]byte{}}
	provider, err := New(startBroker(t, impl), "secret-token")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = provider.Close() }()

	if err := provider.Set("run:1", []byte("payload")); err != nil {
		t.Fatalf("set: %v", err)
	}
	value, err := provider.Get("run:1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(value) != "payload" {
		t.Fatalf("round trip returned %q", value)
	}
	keys, err := provider.List("run:")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 || keys[0] != "run:1" {
		t.Fatalf("list returned %v", keys)
	}
	if err := provider.Delete("run:1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	missing, err := provider.Get("run:1")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("a deleted key still reads back %q", missing)
	}

	impl.mu.Lock()
	defer impl.mu.Unlock()
	if len(impl.tokens) != 5 {
		t.Fatalf("expected one token per call, got %d", len(impl.tokens))
	}
	for i, token := range impl.tokens {
		if token != "secret-token" {
			t.Fatalf("call %d arrived without the operator token (got %q)", i, token)
		}
	}
}

// TestProviderSurfacesBrokerErrors keeps a broker refusal from looking like an
// empty value: a caller that cannot tell "denied" from "absent" would treat an
// unauthorised read as a missing key.
func TestProviderSurfacesBrokerErrors(t *testing.T) {
	impl := &broker{data: map[string][]byte{}, fail: "vault token rejected"}
	provider, err := New(startBroker(t, impl), "wrong")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = provider.Close() }()

	_, err = provider.Get("run:1")
	if err == nil {
		t.Fatal("a refused call must not read as a missing key")
	}
	if !strings.Contains(err.Error(), "Storage.Get") {
		t.Fatalf("the error must name the failed method, got %q", err)
	}
	if !strings.Contains(err.Error(), "vault token rejected") {
		t.Fatalf("the error must carry the broker's reason, got %q", err)
	}
}

// TestNewFailsOnAnUnreachableBroker keeps a missing runtime from hanging: the
// dial is bounded and the address is reported.
func TestNewFailsOnAnUnreachableBroker(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	start := time.Now()
	if _, err := New(addr, "token"); err == nil {
		t.Fatal("connecting to a closed port must fail")
	} else if !strings.Contains(err.Error(), addr) {
		t.Fatalf("the error must name the address, got %q", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("the dial must be bounded, took %s", elapsed)
	}
}
