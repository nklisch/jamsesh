package discovery

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// defaultSAMountPath is the standard Kubernetes service-account volume mount
	// path injected by the kubelet.
	defaultSAMountPath = "/var/run/secrets/kubernetes.io/serviceaccount"

	// defaultInClusterAPIServer is the in-cluster API server address that every
	// Kubernetes cluster exposes via the kubernetes.default.svc DNS name.
	defaultInClusterAPIServer = "https://kubernetes.default.svc"

	// inClusterAPIServerHostEnv is the env var that the kubelet sets when the
	// pod is running inside a cluster. Its presence is the canonical signal that
	// we are in-cluster.
	inClusterAPIServerHostEnv = "KUBERNETES_SERVICE_HOST"
)

// K8sInClusterOptions carries the overridable knobs for K8sInCluster. All
// fields have sensible in-cluster defaults so callers only need to set the
// fields that differ from the defaults. The namespace/service/port fields from
// K8sConfig are passed separately at the call site.
type K8sInClusterOptions struct {
	// MountPath is the directory containing the service-account files:
	//   ca.crt   — cluster CA certificate
	//   token    — bearer token (may rotate; read per request)
	//
	// Defaults to /var/run/secrets/kubernetes.io/serviceaccount. Override in
	// tests to point at a temp directory containing synthetic files.
	MountPath string

	// APIServerURL overrides the discovered API server URL. When empty and
	// KUBERNETES_SERVICE_HOST is set, K8sInCluster uses
	// https://kubernetes.default.svc. When empty and KUBERNETES_SERVICE_HOST is
	// not set, K8sInCluster returns an error directing the caller to set
	// KubeAPIServerURL explicitly.
	APIServerURL string

	// Namespace, ServiceName, PortName, PodPort, ResyncInterval mirror the
	// corresponding fields on K8sConfig and are copied directly into the
	// returned K8sConfig.
	Namespace      string
	ServiceName    string
	PortName       string
	PodPort        int
	ResyncInterval time.Duration // passed through to K8sConfig as-is; zero inherits K8s default
}

// K8sInCluster constructs a K8sConfig suitable for in-cluster use by reading
// the service-account CA cert and bearer token from the kubelet-injected mount
// path (default /var/run/secrets/kubernetes.io/serviceaccount).
//
// Token rotation: rather than reading the token once at startup (which would
// require a restart after every rotation), K8sInCluster wraps the HTTP
// transport in a roundTripperFunc that re-reads the token file on every
// request. Kubernetes rotates projected service-account tokens every few hours;
// re-reading per request is simpler than a goroutine-based refresh and keeps
// the watcher alive across rotations without any coordination overhead.
//
// The returned K8sConfig has BearerToken left empty — token injection is
// handled transparently by the HTTP transport wrapper. Callers should not set
// BearerToken on the returned config; doing so would result in double-injection.
//
// If MountPath does not contain ca.crt or token, K8sInCluster returns a
// structured error naming the missing file. There is intentionally no fallback
// to unauthenticated or unencrypted access — a missing mount in kubernetes mode
// is always a misconfiguration that must fail loudly.
func K8sInCluster(opts K8sInClusterOptions) (K8sConfig, error) {
	mountPath := opts.MountPath
	if mountPath == "" {
		mountPath = defaultSAMountPath
	}

	// Resolve the API server URL.
	apiServerURL := opts.APIServerURL
	if apiServerURL == "" {
		if os.Getenv(inClusterAPIServerHostEnv) != "" {
			apiServerURL = defaultInClusterAPIServer
		} else {
			return K8sConfig{}, fmt.Errorf(
				"k8s in-cluster: KUBERNETES_SERVICE_HOST is not set and no explicit APIServerURL provided; "+
					"set JAMSESH_ROUTER_KUBE_API_SERVER_URL or run inside a Kubernetes pod",
			)
		}
	}

	// Load the cluster CA cert.
	caPath := filepath.Join(mountPath, "ca.crt")
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		if os.IsNotExist(err) {
			return K8sConfig{}, fmt.Errorf("k8s in-cluster: missing file %s: %w", caPath, err)
		}
		return K8sConfig{}, fmt.Errorf("k8s in-cluster: read CA cert %s: %w", caPath, err)
	}

	// Verify the token file exists and is readable; we don't store the content
	// here because we re-read it on every request for rotation transparency.
	tokenPath := filepath.Join(mountPath, "token")
	if _, err := os.Stat(tokenPath); err != nil {
		if os.IsNotExist(err) {
			return K8sConfig{}, fmt.Errorf("k8s in-cluster: missing file %s: %w", tokenPath, err)
		}
		return K8sConfig{}, fmt.Errorf("k8s in-cluster: stat token %s: %w", tokenPath, err)
	}

	// Build a cert pool with the cluster CA.
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return K8sConfig{}, fmt.Errorf("k8s in-cluster: %s contained no valid PEM certificates", caPath)
	}

	// Build the base transport with the cluster CA trusted.
	baseTr := http.DefaultTransport.(*http.Transport).Clone()
	baseTr.TLSClientConfig = &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	}

	// Wrap the transport to inject the (possibly-rotated) bearer token on every
	// request. The token file is re-read per request; the overhead is a single
	// file read on each Endpoints list/watch call, which is negligible compared
	// to the network round-trip.
	tokenRT := &tokenInjectingRoundTripper{
		base:      baseTr,
		tokenPath: tokenPath,
	}

	client := &http.Client{Transport: tokenRT}

	return K8sConfig{
		APIServerURL:        apiServerURL,
		Namespace:           opts.Namespace,
		ServiceName:         opts.ServiceName,
		PortName:            opts.PortName,
		PodPort:             opts.PodPort,
		ResyncInterval:      opts.ResyncInterval,
		HTTPClient:          client,
		// BearerToken is intentionally empty — injection is handled by
		// tokenInjectingRoundTripper so the token is always current.
	}, nil
}

// tokenInjectingRoundTripper wraps an http.RoundTripper and injects a
// "Authorization: Bearer <token>" header read fresh from tokenPath on every
// request. This design tolerates Kubernetes projected service-account token
// rotation without any goroutine coordination.
type tokenInjectingRoundTripper struct {
	base      http.RoundTripper
	tokenPath string
}

// RoundTrip reads the token file, injects it as a Bearer header, then
// delegates to the base transport. If the token file is unreadable the request
// is aborted with an error rather than falling back to no-auth (which would
// likely result in a 401 from the API server — fail loudly here instead).
func (t *tokenInjectingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	raw, err := os.ReadFile(t.tokenPath)
	if err != nil {
		return nil, fmt.Errorf("k8s in-cluster: read bearer token %s: %w", t.tokenPath, err)
	}
	token := strings.TrimRight(string(raw), " \t\r\n")

	// Clone the request so we do not mutate the caller's headers.
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(clone)
}
