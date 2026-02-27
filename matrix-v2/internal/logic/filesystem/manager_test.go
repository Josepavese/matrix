package filesystem

import (
	"errors"
	"testing"

	"github.com/jose/matrix-v2/internal/middleware"
)

// MockFS implemets middleware.FS for testing
type MockFS struct {
	CreateDirCalled bool
	Mounted         bool
}

func (m *MockFS) CreateDirectory(path string) error {
	m.CreateDirCalled = true
	if path == "/fail" {
		return errors.New("creation failed")
	}
	return nil
}

func (m *MockFS) Mount(dir string) error {
	m.Mounted = true
	return nil
}

func (m *MockFS) Unmount() error {
	m.Mounted = false
	return nil
}

func TestManager_MountVirtualFS(t *testing.T) {
	mock := &MockFS{}
	mgr := NewManager(mock)

	// Test success
	err := mgr.MountVirtualFS("/tmp/matrix")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !mock.CreateDirCalled {
		t.Errorf("Expected CreateDirectory to be called")
	}

	// Test failure propagation
	mock.CreateDirCalled = false
	err = mgr.MountVirtualFS("/fail")
	if err == nil {
		t.Errorf("Expected error for /fail path")
	}
	var midErr *middleware.Error
	if !errors.As(err, &midErr) {
		t.Errorf("Expected middleware.Error, got %T", err)
	}
}
