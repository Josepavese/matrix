package network

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadRefusesHTTPSRedirectToHTTP(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("should not download"))
	}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.RedirectHandler(plain.URL+"/artifact", http.StatusFound))
	defer secure.Close()
	provider := NewProvider()
	provider.downloadClient.Transport = secure.Client().Transport
	target := filepath.Join(t.TempDir(), "artifact")
	err := provider.Download(context.Background(), secure.URL+"/artifact", target)
	if err == nil || !strings.Contains(err.Error(), "refusing HTTPS redirect") {
		t.Fatalf("downgrade accepted: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("download wrote artifact after downgrade: %v", err)
	}
}

func TestFetchJSONRefusesHTTPSRedirectToHTTP(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"agents":[]}`))
	}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.RedirectHandler(plain.URL+"/registry.json", http.StatusFound))
	defer secure.Close()
	provider := NewProvider()
	provider.httpClient.Transport = secure.Client().Transport
	var index map[string]interface{}
	err := provider.FetchJSON(context.Background(), secure.URL+"/registry.json", &index)
	if err == nil || !strings.Contains(err.Error(), "refusing HTTPS redirect") {
		t.Fatalf("registry index downgrade accepted: %v", err)
	}
}

// TestDownloadRejectsAnUnsupportedScheme keeps a caller from being told a
// download succeeded when no request was ever made.
func TestDownloadRejectsAnUnsupportedScheme(t *testing.T) {
	provider := NewProvider()
	dest := filepath.Join(t.TempDir(), "out.bin")
	for _, raw := range []string{"", "not a url", "file:///etc/passwd", "ftp://example.test/f"} {
		if err := provider.Download(context.Background(), raw, dest); err == nil {
			t.Fatalf("url %q must be refused", raw)
		}
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("a refused download must not leave a partial file")
	}
}

// TestDownloadReportsAnHTTPErrorInsteadOfWritingIt is the failure that matters
// most: an error page must never be stored as if it were the artefact.
func TestDownloadReportsAnHTTPErrorInsteadOfWritingIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	err := NewProvider().Download(context.Background(), server.URL+"/missing", dest)
	if err == nil {
		t.Fatal("a 404 must be reported as an error")
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Fatal("the error body must not be written to the destination")
	}
}

// TestDownloadWritesTheBodyOnSuccess is the positive contract, including the
// parent directory the caller did not create.
func TestDownloadWritesTheBodyOnSuccess(t *testing.T) {
	payload := []byte("contenuto")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "nested", "out.bin")
	if err := NewProvider().Download(context.Background(), server.URL+"/file", dest); err != nil {
		t.Fatalf("download: %v", err)
	}
	written, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(written) != string(payload) {
		t.Fatalf("downloaded %q, want %q", written, payload)
	}
}

// TestDownloadHonoursContextCancellation keeps a cancelled run from leaving a
// request in flight and a partial file behind.
func TestDownloadHonoursContextCancellation(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = w.Write([]byte("late"))
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	dest := filepath.Join(t.TempDir(), "out.bin")
	if err := NewProvider().Download(ctx, server.URL+"/slow", dest); err == nil {
		t.Fatal("a cancelled download must be reported")
	}
}

// TestCanDialDistinguishesOpenAndClosedPorts keeps a reachability check from
// answering yes for a port nobody listens on.
func TestCanDialDistinguishesOpenAndClosedPorts(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	provider := NewProvider()
	if !provider.CanDial(address) {
		t.Fatal("an open listener must be reachable")
	}
	_ = listener.Close()
	if provider.CanDial(address) {
		t.Fatal("a closed port must not be reported as reachable")
	}
	if provider.CanDial("not-an-address") {
		t.Fatal("a malformed address must not be reported as reachable")
	}
}

// TestDownloadTruncatesLongErrorMessages keeps a huge error body from filling the
// log or the terminal through the returned error.
func TestDownloadTruncatesLongErrorMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("e", 20000)))
	}))
	defer server.Close()

	err := NewProvider().Download(context.Background(), server.URL+"/big", filepath.Join(t.TempDir(), "out.bin"))
	if err == nil {
		t.Fatal("a 500 must be reported")
	}
	if len(err.Error()) > 4096 {
		t.Fatalf("the error message carries %d bytes; it must be bounded", len(err.Error()))
	}
}
