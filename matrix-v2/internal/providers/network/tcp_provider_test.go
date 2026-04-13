package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewProvider_Clients(t *testing.T) {
	p := NewProvider()
	if p.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
	if p.downloadClient == nil {
		t.Error("downloadClient should not be nil")
	}
	if p.httpClient.Timeout != 30*time.Second {
		t.Errorf("httpClient timeout = %v, want 30s", p.httpClient.Timeout)
	}
	if p.downloadClient.Timeout != downloadTimeout {
		t.Errorf("downloadClient timeout = %v, want %v", p.downloadClient.Timeout, downloadTimeout)
	}
}

func TestGetFreePort(t *testing.T) {
	p := NewProvider()
	port, err := p.GetFreePort()
	if err != nil {
		t.Fatalf("GetFreePort: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Errorf("invalid port: %d", port)
	}
}

func TestCanDial_OpenPort(t *testing.T) {
	p := NewProvider()
	l, err := p.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("can't open listener: %v", err)
	}
	defer func() { _ = l.Close() }()

	addr := l.Addr().String()
	if !p.CanDial(addr) {
		t.Errorf("CanDial should return true for %s", addr)
	}
}

func TestCanDial_ClosedPort(t *testing.T) {
	p := NewProvider()
	if p.CanDial("1.1.1.1:1") {
		t.Error("CanDial should return false for unreachable port")
	}
}

func TestFetch_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer ts.Close()

	p := NewProvider()
	data, err := p.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %s", string(data))
	}
}

func TestFetch_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	p := NewProvider()
	_, err := p.Fetch(context.Background(), ts.URL)
	if err == nil {
		t.Error("expected error for 404")
	}
}

func TestPostJSON_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected JSON content type")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	p := NewProvider()
	data, code, err := p.PostJSON(context.Background(), ts.URL, map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if code != 200 {
		t.Errorf("expected 200, got %d", code)
	}
	if string(data) != `{"ok":true}` {
		t.Errorf("unexpected body: %s", string(data))
	}
}

func TestDownload_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("binary-data"))
	}))
	defer ts.Close()

	p := NewProvider()
	tmpFile := t.TempDir() + "/downloaded.bin"
	if err := p.Download(context.Background(), ts.URL, tmpFile); err != nil {
		t.Fatalf("Download: %v", err)
	}
}

func TestDownload_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p := NewProvider()
	tmpFile := t.TempDir() + "/failed.bin"
	err := p.Download(context.Background(), ts.URL, tmpFile)
	if err == nil {
		t.Error("expected error for 500")
	}
}
