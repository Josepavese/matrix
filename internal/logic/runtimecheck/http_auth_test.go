package runtimecheck

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type authenticatedReportNet struct {
	*localReportNet
	headers    map[string]string
	plainCalls int
	failure    error
}

func (n *authenticatedReportNet) FetchJSON(ctx context.Context, url string, target interface{}) error {
	n.plainCalls++
	return n.localReportNet.FetchJSON(ctx, url, target)
}
func (n *authenticatedReportNet) FetchJSONWithHeaders(_ context.Context, _ string, headers map[string]string, target interface{}) error {
	n.headers = headers
	if n.failure != nil {
		return n.failure
	}
	value, ok := target.(*map[string]any)
	if !ok {
		return errors.New("unexpected target")
	}
	*value = n.report
	return nil
}
func validAuthenticatedRuntime() map[string]any {
	return map[string]any{"vault_exists": true, "jsonrpc_daemon_up": true, "matrix_http_up": true, "a2a_http_up": true, "matrix_http_authenticated": true, "warnings": []string{}}
}
func TestLocalReportUsesConfiguredRuntimeCredential(t *testing.T) {
	net := &authenticatedReportNet{localReportNet: &localReportNet{canDial: true, report: validAuthenticatedRuntime()}}
	report, err := BuildLocalReport(LocalInput{RuntimeAPIKey: "probe-test-key", MatrixHTTPAddr: "127.0.0.1:9091", Net: net, FS: &localReportFS{}})
	if err != nil {
		t.Fatal(err)
	}
	if net.headers["X-Matrix-Key"] != "probe-test-key" || net.plainCalls != 0 {
		t.Fatal("authenticated probe omitted credential or retried anonymously")
	}
	if warningContains(report, "fallback") {
		t.Fatal("authenticated runtime report was discarded")
	}
}
func TestRuntimeProbeRejectsBackendWithoutAuthenticationSupport(t *testing.T) {
	_, err := FetchRuntimeReport(context.Background(), &localReportNet{}, "http://127.0.0.1/runtime", "probe-test-key")
	if err == nil || !strings.Contains(err.Error(), "authenticated requests") {
		t.Fatalf("credential silently discarded: %v", err)
	}
	if strings.Contains(err.Error(), "probe-test-key") {
		t.Fatal("credential appeared in diagnostic")
	}
}
func TestRuntimeProbePreservesAuthenticationFailures(t *testing.T) {
	net := &authenticatedReportNet{localReportNet: &localReportNet{}, failure: errors.New("unexpected status code: 401")}
	_, err := FetchRuntimeReport(context.Background(), net, "http://127.0.0.1/runtime", "probe-test-key")
	if err == nil || !strings.Contains(err.Error(), "401") || net.plainCalls != 0 {
		t.Fatalf("authentication failure hidden: %v", err)
	}
}
