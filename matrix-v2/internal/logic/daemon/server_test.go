package daemon

import (
	"context"
	"net"
	"testing"

	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/middleware"
)

// MockNetwork implements middleware.Network
type MockNetwork struct {
	listenCalled chan struct{}
}

func newMockNetwork() *MockNetwork {
	return &MockNetwork{listenCalled: make(chan struct{})}
}

func (m *MockNetwork) Download(ctx context.Context, url, destPath string) error {
	return nil
}

func (m *MockNetwork) FetchJSON(ctx context.Context, url string, target interface{}) error {
	return nil
}

func (m *MockNetwork) Listen(network, address string) (middleware.ClosableListener, error) {
	close(m.listenCalled)
	return &MockListener{done: make(chan struct{})}, nil
}

func (m *MockNetwork) GetFreePort() (int, error) {
	return 8080, nil
}
func (m *MockNetwork) Fetch(ctx context.Context, url string) ([]byte, error) {
	return nil, nil
}
func (m *MockNetwork) PostJSON(ctx context.Context, url string, body interface{}) ([]byte, int, error) {
	return nil, 0, nil
}
func (m *MockNetwork) CanDial(address string) bool { return false }

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
