package fusefs

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestMountRejectsMissingDriverAndOccupiedDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "original"), []byte("PRESERVE"), 0600); err != nil {
		t.Fatal(err)
	}
	provider := NewProvider().WithView(fstest.MapFS{}).WithDriverPath(filepath.Join(t.TempDir(), "missing-driver"))
	if err := provider.Mount(dir); err == nil {
		t.Fatal("existing data hidden")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "original"))
	if string(data) != "PRESERVE" {
		t.Fatal("original changed")
	}
	if err := provider.Mount(t.TempDir()); err == nil {
		t.Fatal("missing prerequisite silently accepted")
	}
	if err := provider.Unmount(); err != nil {
		t.Fatal("failed startup left mount state", err)
	}
}

func TestProjectionRejectsUnauthenticatedAndMutationRequests(t *testing.T) {
	view := fstest.MapFS{"status.json": &fstest.MapFile{Data: []byte("VISIBLE"), Mode: 0400}}
	handler := projectionHandler(view, "private-projection-key")
	for _, tc := range []struct {
		method, key string
		code        int
	}{{"GET", "", 401}, {"GET", "wrong", 401}, {"POST", "private-projection-key", 405}, {"GET", "private-projection-key", 200}} {
		request := httptest.NewRequest(tc.method, "http://loopback/status.json", nil)
		request.Header.Set("X-Matrix-FS-Key", tc.key)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.code {
			t.Fatal(response.Code, tc)
		}
		if tc.code == http.StatusOK {
			data, _ := io.ReadAll(response.Result().Body)
			if string(data) != "VISIBLE" || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private projection contract")
			}
		}
	}
	if strings.Contains(strings.Join(mountArgs("/mount", "/private/config"), " "), "private-projection-key") {
		t.Fatal("projection token in argv")
	}
}
