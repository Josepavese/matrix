//go:build windows

package exec_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- Windows certificate store thumbprints use SHA-1 identifiers.
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	goexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josepavese/matrix/internal/middleware"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
)

const (
	trustChildEnv = "MATRIX_WINDOWS_TRUST_CHILD"
	trustURLEnv   = "MATRIX_WINDOWS_TRUST_URL"
)

func TestWindowsSystemTrustChild(t *testing.T) {
	if os.Getenv(trustChildEnv) != "1" {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(os.Getenv(trustURLEnv)) // #nosec G107 -- URL belongs to the parent test server.
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %s", response.Status)
	}
}

func TestStartPipedPreservesWindowsCurrentUserTrust(t *testing.T) {
	caDER, serverCertificate := createWindowsTrustFixture(t)
	caPath := filepath.Join(t.TempDir(), "matrix-acp-test-root.cer")
	if err := os.WriteFile(caPath, caDER, 0o600); err != nil {
		t.Fatal(err)
	}
	fingerprintBytes := sha1.Sum(caDER) // #nosec G401 -- Windows store deletion requires the SHA-1 thumbprint.
	fingerprint := strings.ToUpper(hex.EncodeToString(fingerprintBytes[:]))
	runPowerShell(t, []string{"MATRIX_TEST_CA=" + caPath},
		`Import-Certificate -FilePath $env:MATRIX_TEST_CA -CertStoreLocation Cert:\CurrentUser\Root | Out-Null`)
	t.Cleanup(func() {
		runPowerShell(t, []string{"MATRIX_TEST_THUMBPRINT=" + fingerprint},
			`Remove-Item -LiteralPath ("Cert:\CurrentUser\Root\" + $env:MATRIX_TEST_THUMBPRINT) -Force`)
	})

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{serverCertificate}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()

	childArgs := []string{"-test.run=^TestWindowsSystemTrustChild$", "-test.count=1"}
	childEnv := []string{trustChildEnv + "=1", trustURLEnv + "=" + server.URL, "MATRIX_ACP_ENV_PROBE=present"}
	directCtx, cancelDirect := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelDirect()
	direct := goexec.CommandContext(directCtx, os.Args[0], childArgs...)
	direct.Env = append(os.Environ(), childEnv...)
	if output, err := direct.CombinedOutput(); err != nil {
		t.Fatalf("direct Windows trust control failed: %v: %s", err, output)
	}

	process, err := execprovider.NewProvider().StartPiped(middleware.CommandSpec{
		Runner: os.Args[0], Args: childArgs, Env: childEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	output, readErr := io.ReadAll(process.Stdout())
	waitErr := process.Wait()
	if readErr != nil || waitErr != nil {
		t.Fatalf("MATRIX terminal launch lost Windows trust: read=%v wait=%v output=%s", readErr, waitErr, output)
	}
}

func createWindowsTrustFixture(t *testing.T) ([]byte, tls.Certificate) {
	t.Helper()
	rootKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	root := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "MATRIX ACP Windows trust test root"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, root, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return rootDER, certificate
}

func runPowerShell(t *testing.T, env []string, script string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := goexec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	command.Env = append(os.Environ(), env...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell certificate store command: %v: %s", err, output)
	}
}
