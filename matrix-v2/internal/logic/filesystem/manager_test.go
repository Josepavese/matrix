package filesystem

import (
	"errors"
	"os"
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

func (m *MockFS) RemoveAll(path string) error                  { return nil }
func (m *MockFS) Stat(path string) (os.FileInfo, error)        { return nil, nil }
func (m *MockFS) MkdirAll(path string, perm os.FileMode) error { return nil }
func (m *MockFS) UserHomeDir() (string, error)                 { return "/home/test", nil }
func (m *MockFS) TempDir() string                              { return "/tmp" }
func (m *MockFS) Open(path string) (middleware.File, error)    { return nil, nil }
func (m *MockFS) OpenFile(path string, flag int, perm os.FileMode) (middleware.File, error) {
	return nil, nil
}
func (m *MockFS) ReadFile(path string) ([]byte, error) { return nil, nil }
func (m *MockFS) Remove(path string) error             { return nil }
func (m *MockFS) Rename(oldpath, newpath string) error { return nil }

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
