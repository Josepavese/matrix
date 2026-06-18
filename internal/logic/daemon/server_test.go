package daemon

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/Josepavese/matrix/internal/logic/memstore"
	"github.com/Josepavese/matrix/internal/logic/vault"
	"github.com/Josepavese/matrix/internal/middleware"
)

// MockNetwork implements middleware.Network
type MockNetwork struct {
	listenCalled chan struct{}
}

func newMockNetwork() *MockNetwork {
	return &MockNetwork{listenCalled: make(chan struct{})}
}

func (m *MockNetwork) Download(_ context.Context, _, _ string) error {
	return nil
}

func (m *MockNetwork) FetchJSON(_ context.Context, _ string, _ interface{}) error {
	return nil
}

func (m *MockNetwork) Listen(_, _ string) (middleware.ClosableListener, error) {
	close(m.listenCalled)
	return &MockListener{done: make(chan struct{})}, nil
}

func (m *MockNetwork) GetFreePort() (int, error) {
	return 8080, nil
}
func (m *MockNetwork) Fetch(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (m *MockNetwork) PostJSON(_ context.Context, _ string, _ interface{}) ([]byte, int, error) {
	return nil, 0, nil
}
func (m *MockNetwork) CanDial(_ string) bool { return false }

// MockListener implements middleware.ClosableListener
type MockListener struct {
	done chan struct{}
}

func (m *MockListener) Accept() (net.Conn, error) {
	<-m.done
	return nil, net.ErrClosed
}
func (m *MockListener) Close() error {
	select {
	case <-m.done:
	default:
		close(m.done)
	}
	return nil
}
func (m *MockListener) Addr() net.Addr { return nil }

func TestServer_Start(t *testing.T) {
	mockNet := newMockNetwork()
	srv := NewServer(&vault.Vault{}, mockNet)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = srv.Start(ctx, ":0")
	}()

	<-mockNet.listenCalled

	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestVaultServiceRequiresAPIKeyWhenConfigured(t *testing.T) {
	service := NewVaultService(vault.NewVault(memstore.New()), "secret")

	var setReply VaultReply
	if err := service.Set(&VaultArgs{Key: "k", Value: "v"}, &setReply); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expected ErrInvalidAPIKey without api key, got %v", err)
	}
	if err := service.Set(&VaultArgs{Key: "k", Value: "v", APIKey: "secret"}, &setReply); err != nil {
		t.Fatalf("set with api key failed: %v", err)
	}

	var getReply VaultReply
	if err := service.Get(&VaultArgs{Key: "k"}, &getReply); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expected ErrInvalidAPIKey on get without api key, got %v", err)
	}
	if err := service.Get(&VaultArgs{Key: "k", APIKey: "secret"}, &getReply); err != nil {
		t.Fatalf("get with api key failed: %v", err)
	}
	if getReply.Value != "v" {
		t.Fatalf("expected vault value v, got %q", getReply.Value)
	}
}

func TestAuthServiceRejectsNilArgs(t *testing.T) {
	service := &AuthService{apiKey: "secret"}
	var reply AuthReply
	if err := service.Authenticate(nil, &reply); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expected ErrInvalidAPIKey for nil auth args, got %v", err)
	}
	if reply.Success {
		t.Fatalf("nil auth args should not authenticate")
	}
}
