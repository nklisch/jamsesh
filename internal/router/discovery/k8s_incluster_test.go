package discovery

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// makeSelfSignedCA generates a minimal self-signed CA certificate and returns
// the PEM-encoded cert and the DER-encoded cert+key pair for use in TLS servers.
func makeSelfSignedCA(t *testing.T) (caPEM []byte, caConfig *tls.Certificate) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}

	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
	return caPEM, &tlsCert
}

// makeServerCert generates a server certificate signed by the given CA cert+key.
func makeServerCert(t *testing.T, caTLSCert *tls.Certificate) tls.Certificate {
	t.Helper()

	caCert, err := x509.ParseCertificate(caTLSCert.Certificate[0])
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}
	caKey := caTLSCert.PrivateKey.(*ecdsa.PrivateKey)

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server cert: %v", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  serverKey,
	}
}

// writeSAMount writes ca.crt and token files to dir for use as a synthetic
// service-account mount path.
func writeSAMount(t *testing.T, dir string, caPEM []byte, token string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "ca.crt"), caPEM, 0600); err != nil {
		t.Fatalf("write ca.crt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(token), 0600); err != nil {
		t.Fatalf("write token: %v", err)
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestK8sInCluster_TLSAndToken verifies that:
//  1. K8sInCluster returns a non-nil HTTPClient whose transport trusts the CA
//     cert from the mount path.
//  2. Requests made through that client carry "Authorization: Bearer <token>".
func TestK8sInCluster_TLSAndToken(t *testing.T) {
	t.Setenv(inClusterAPIServerHostEnv, "10.96.0.1") // simulate in-cluster env

	// Generate a self-signed CA and a server cert signed by it.
	caPEM, caTLSCert := makeSelfSignedCA(t)
	serverCert := makeServerCert(t, caTLSCert)

	// Start a TLS test server using the server cert.
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Auth", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	// Write synthetic SA mount.
	mountDir := t.TempDir()
	const token = "test-bearer-token-abc123"
	writeSAMount(t, mountDir, caPEM, token)

	// Build K8sConfig via K8sInCluster.
	cfg, err := K8sInCluster(K8sInClusterOptions{
		MountPath:   mountDir,
		APIServerURL: srv.URL,
		Namespace:   "jamsesh",
		ServiceName: "portal",
		PodPort:     8443,
	})
	if err != nil {
		t.Fatalf("K8sInCluster: %v", err)
	}

	if cfg.HTTPClient == nil {
		t.Fatal("K8sInCluster: HTTPClient is nil")
	}

	// Verify the transport has RootCAs set.
	tr, ok := cfg.HTTPClient.Transport.(*tokenInjectingRoundTripper)
	if !ok {
		t.Fatalf("expected *tokenInjectingRoundTripper, got %T", cfg.HTTPClient.Transport)
	}
	baseTr, ok := tr.base.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport as base, got %T", tr.base)
	}
	if baseTr.TLSClientConfig == nil || baseTr.TLSClientConfig.RootCAs == nil {
		t.Fatal("K8sInCluster: TLSClientConfig.RootCAs is nil")
	}

	// Make a request and verify the Authorization header is injected.
	resp, err := cfg.HTTPClient.Get(srv.URL + "/ping")
	if err != nil {
		t.Fatalf("GET via K8sInCluster client: %v", err)
	}
	defer resp.Body.Close()

	got := resp.Header.Get("X-Auth")
	want := "Bearer " + token
	if got != want {
		t.Errorf("Authorization header: got %q, want %q", got, want)
	}
}

// TestK8sInCluster_TokenRotation verifies that when the token file is updated
// between requests, the second request carries the new token. This is the core
// property of the read-on-request rotation design.
func TestK8sInCluster_TokenRotation(t *testing.T) {
	t.Setenv(inClusterAPIServerHostEnv, "10.96.0.1")

	caPEM, caTLSCert := makeSelfSignedCA(t)
	serverCert := makeServerCert(t, caTLSCert)

	// Collect the Authorization header from each request.
	var received []string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	mountDir := t.TempDir()
	const token1 = "first-token"
	writeSAMount(t, mountDir, caPEM, token1)

	cfg, err := K8sInCluster(K8sInClusterOptions{
		MountPath:   mountDir,
		APIServerURL: srv.URL,
		Namespace:   "jamsesh",
		ServiceName: "portal",
		PodPort:     8443,
	})
	if err != nil {
		t.Fatalf("K8sInCluster: %v", err)
	}

	// First request with token1.
	resp, err := cfg.HTTPClient.Get(srv.URL + "/ping")
	if err != nil {
		t.Fatalf("first GET: %v", err)
	}
	resp.Body.Close()

	// Rotate the token by overwriting the file.
	const token2 = "rotated-token"
	if err := os.WriteFile(filepath.Join(mountDir, "token"), []byte(token2), 0600); err != nil {
		t.Fatalf("rotate token: %v", err)
	}

	// Second request — should use token2.
	resp, err = cfg.HTTPClient.Get(srv.URL + "/ping")
	if err != nil {
		t.Fatalf("second GET: %v", err)
	}
	resp.Body.Close()

	if len(received) != 2 {
		t.Fatalf("expected 2 requests captured, got %d", len(received))
	}
	if received[0] != "Bearer "+token1 {
		t.Errorf("request 1: got %q, want %q", received[0], "Bearer "+token1)
	}
	if received[1] != "Bearer "+token2 {
		t.Errorf("request 2: got %q, want %q", received[1], "Bearer "+token2)
	}
}

// TestK8sInCluster_MissingCA verifies that K8sInCluster returns an error that
// names the missing file when ca.crt is absent from the mount path.
func TestK8sInCluster_MissingCA(t *testing.T) {
	t.Setenv(inClusterAPIServerHostEnv, "10.96.0.1")

	mountDir := t.TempDir()
	// Write only the token file — deliberately omit ca.crt.
	if err := os.WriteFile(filepath.Join(mountDir, "token"), []byte("tok"), 0600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	_, err := K8sInCluster(K8sInClusterOptions{
		MountPath:   mountDir,
		Namespace:   "jamsesh",
		ServiceName: "portal",
		PodPort:     8443,
	})
	if err == nil {
		t.Fatal("expected error for missing ca.crt, got nil")
	}
	if want := "ca.crt"; !containsStr(err.Error(), want) {
		t.Errorf("error %q does not mention %q", err.Error(), want)
	}
}

// TestK8sInCluster_MissingToken verifies that K8sInCluster returns an error
// that names the missing file when token is absent from the mount path.
func TestK8sInCluster_MissingToken(t *testing.T) {
	t.Setenv(inClusterAPIServerHostEnv, "10.96.0.1")

	caPEM, _ := makeSelfSignedCA(t)

	mountDir := t.TempDir()
	// Write only ca.crt — deliberately omit the token file.
	if err := os.WriteFile(filepath.Join(mountDir, "ca.crt"), caPEM, 0600); err != nil {
		t.Fatalf("write ca.crt: %v", err)
	}

	_, err := K8sInCluster(K8sInClusterOptions{
		MountPath:   mountDir,
		Namespace:   "jamsesh",
		ServiceName: "portal",
		PodPort:     8443,
	})
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}
	if want := "token"; !containsStr(err.Error(), want) {
		t.Errorf("error %q does not mention %q", err.Error(), want)
	}
}

// TestK8sInCluster_NoAPIServerURL verifies that when no APIServerURL is given
// and KUBERNETES_SERVICE_HOST is not set, K8sInCluster returns a clear error
// directing the operator to provide explicit config.
func TestK8sInCluster_NoAPIServerURL(t *testing.T) {
	// Explicitly clear the env var so the in-cluster auto-detection does not fire.
	t.Setenv(inClusterAPIServerHostEnv, "")

	caPEM, _ := makeSelfSignedCA(t)
	mountDir := t.TempDir()
	writeSAMount(t, mountDir, caPEM, "token")

	_, err := K8sInCluster(K8sInClusterOptions{
		MountPath:   mountDir,
		Namespace:   "jamsesh",
		ServiceName: "portal",
		PodPort:     8443,
	})
	if err == nil {
		t.Fatal("expected error when KUBERNETES_SERVICE_HOST unset and no APIServerURL, got nil")
	}
}

// containsStr is a simple substring check used in error-message assertions.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
