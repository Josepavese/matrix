// Package network provides TCP/HTTP networking primitives.
package network

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
)

// maxResponseBody is the safety limit for HTTP response bodies (50 MB).
// Responses exceeding this will be truncated with a warning.
const maxResponseBody = 50 * 1024 * 1024

// downloadTimeout is the timeout for large binary downloads (5 minutes).
const downloadTimeout = 5 * time.Minute

const maxHTTPAttempts = 3

// Provider implements middleware.Network using standard TCP
type Provider struct {
	httpClient     *http.Client
	downloadClient *http.Client
}

// NewProvider returns a new TCP network provider
func NewProvider() *Provider {
	return &Provider{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		downloadClient: &http.Client{
			Timeout: downloadTimeout,
		},
	}
}

// Download fetches a file from a URL and saves it to a local path.
// Uses a dedicated client with a longer timeout suited for large binaries.
func (p *Provider) Download(ctx context.Context, url, destPath string) error {
	var lastErr error
	for attempt := 1; attempt <= maxHTTPAttempts; attempt++ {
		statusCode, err := p.downloadOnce(ctx, url, destPath)
		if err != nil {
			lastErr = err
			if shouldRetryHTTP(ctx, attempt, statusCode, err) {
				continue
			}
			return err
		}
		return nil
	}
	return lastErr
}

func (p *Provider) downloadOnce(ctx context.Context, url, destPath string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := p.downloadClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Ensure destination directory exists
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return resp.StatusCode, err
	}

	out, err := os.Create(destPath)
	if err != nil {
		return resp.StatusCode, err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return resp.StatusCode, err
	}
	return resp.StatusCode, out.Sync()
}

// FetchJSON fetches a JSON resource from a URL and decodes it into the target
func (p *Provider) FetchJSON(ctx context.Context, url string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

// Listen opens a standard net.Listen socket
func (p *Provider) Listen(network, address string) (middleware.ClosableListener, error) {
	return net.Listen(network, address)
}

// GetFreePort asks the OS for an available TCP port safely avoiding collisions.
func (p *Provider) GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	tcpAddr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", l.Addr())
	}
	return tcpAddr.Port, nil
}

// Fetch fetches a URL and returns the response body bytes.
func (p *Provider) Fetch(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= maxHTTPAttempts; attempt++ {
		data, statusCode, err := p.fetchOnce(ctx, url)
		if err != nil {
			lastErr = err
			if shouldRetryHTTP(ctx, attempt, statusCode, err) {
				continue
			}
			return nil, err
		}
		return data, nil
	}
	return nil, lastErr
}

func (p *Provider) fetchOnce(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	return data, resp.StatusCode, err
}

// PostJSON posts a JSON body to a URL and returns the response body bytes and status code.
func (p *Provider) PostJSON(ctx context.Context, url string, body interface{}) ([]byte, int, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// CanDial reports whether a TCP address is reachable.
func (p *Provider) CanDial(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 750*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func shouldRetryHTTP(ctx context.Context, attempt, statusCode int, err error) bool {
	if err == nil || attempt >= maxHTTPAttempts || ctx.Err() != nil {
		return false
	}
	if statusCode != 0 && statusCode != http.StatusTooManyRequests && statusCode < http.StatusInternalServerError {
		return false
	}
	timer := time.NewTimer(time.Duration(attempt) * 250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
