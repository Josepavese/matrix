package filesystem

import (
	"errors"
	"os"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
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

func (m *MockFS) Mount(_ string) error {
	m.Mounted = true
	return nil
}

func (m *MockFS) Unmount() error {
	m.Mounted = false
	return nil
}

func (m *MockFS) RemoveAll(_ string) error               { return nil }
func (m *MockFS) Stat(_ string) (os.FileInfo, error)     { return nil, nil }
func (m *MockFS) MkdirAll(_ string, _ os.FileMode) error { return nil }
func (m *MockFS) UserHomeDir() (string, error)           { return "/home/test", nil }
func (m *MockFS) TempDir() string                        { return "/tmp" }
func (m *MockFS) Open(_ string) (middleware.File, error) { return nil, nil }
func (m *MockFS) OpenFile(_ string, _ int, _ os.FileMode) (middleware.File, error) {
	return nil, nil
}
func (m *MockFS) ReadFile(_ string) ([]byte, error) { return nil, nil }
func (m *MockFS) Remove(_ string) error             { return nil }
func (m *MockFS) Rename(_, _ string) error          { return nil }

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
