package daemon

import (
	"net"
	"testing"
	"time"

	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/middleware"
)

// MockNetwork implements middleware.Network
type MockNetwork struct {
	ListenCalled bool
}

func (m *MockNetwork) Listen(network, address string) (middleware.ClosableListener, error) {
	m.ListenCalled = true
	return &MockListener{done: make(chan struct{})}, nil
}

// MockListener implements middleware.ClosableListener
type MockListener struct {
	done chan struct{}
}

func (m *MockListener) Accept() (net.Conn, error) {
	<-m.done // Block until closed
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
	mockNet := &MockNetwork{}
	srv := NewServer(&vault.Vault{}, mockNet)

	// Start server in a goroutine because it blocks in an infinite loop
	addr := ":9091"
	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Start(addr)
	}()

	// Give it a tiny bit of time to reach the loop
	time.Sleep(50 * time.Millisecond)

	if !mockNet.ListenCalled {
		t.Errorf("Expected network.Listen to be called")
	}

	// Clean up: closing the listener should trigger an error in Accept/Start if implemented for graceful exit,
	// but here we just want to ensure we don't leak goreoutines too badly in tests.
	srv.Stop()
}
