// Package config loads and validates the jamsesh-router configuration.
// Config is sourced from an optional YAML file with env-var overrides.
//
// YAML keys: bind, discovery_mode, static_pods, probe_interval, probe_timeout,
//
//	hint_cache_ttl, vnodes, shutdown_grace_s,
//	kube_api_server_url, kube_namespace, kube_service_name,
//	kube_pod_port, kube_bearer_token, kube_resync_interval_s
//
// Env vars:
//
//	JAMSESH_ROUTER_BIND                  – listen address, default ":8080"
//	JAMSESH_ROUTER_DISCOVERY_MODE        – "static" (default) or "kubernetes"
//	JAMSESH_ROUTER_STATIC_PODS           – comma-separated "host:port" list (static mode)
//	JAMSESH_ROUTER_SHUTDOWN_GRACE_S      – seconds for graceful drain
//	JAMSESH_ROUTER_KUBE_API_SERVER_URL   – k8s API server base URL (kubernetes mode)
//	JAMSESH_ROUTER_KUBE_NAMESPACE        – pod namespace (kubernetes mode)
//	JAMSESH_ROUTER_KUBE_SERVICE_NAME     – Endpoints object name (kubernetes mode)
//	JAMSESH_ROUTER_KUBE_POD_PORT         – port appended to discovered IPs (kubernetes mode)
//	JAMSESH_ROUTER_KUBE_BEARER_TOKEN     – bearer token for API auth (kubernetes mode, optional)
//	JAMSESH_ROUTER_KUBE_RESYNC_INTERVAL_S – re-list interval in seconds (kubernetes mode, default 30)
//
// Remaining knobs (probe_interval, probe_timeout, hint_cache_ttl, vnodes)
// are YAML-only in v1.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DiscoveryMode selects the pod-discovery backend.
type DiscoveryMode string

const (
	// DiscoveryStatic uses a fixed pod list with /readyz probing (default).
	DiscoveryStatic DiscoveryMode = "static"
	// DiscoveryKubernetes watches the Kubernetes Endpoints API.
	DiscoveryKubernetes DiscoveryMode = "kubernetes"
)

// Config holds the full jamsesh-router configuration.
type Config struct {
	// Bind is the TCP address the router listens on. Default: ":8080".
	// Env: JAMSESH_ROUTER_BIND
	Bind string `yaml:"bind"`

	// DiscoveryMode selects the discovery backend: "static" or "kubernetes".
	// Default: "static". Env: JAMSESH_ROUTER_DISCOVERY_MODE
	DiscoveryMode DiscoveryMode `yaml:"discovery_mode"`

	// StaticPods is the fixed list of pod addresses ("host:port").
	// Env: JAMSESH_ROUTER_STATIC_PODS (comma-separated)
	// Required when DiscoveryMode is "static".
	StaticPods []string `yaml:"static_pods"`

	// ── Kubernetes discovery (DiscoveryMode == "kubernetes") ──────────────

	// KubeAPIServerURL is the base URL of the Kubernetes API server.
	// Env: JAMSESH_ROUTER_KUBE_API_SERVER_URL
	KubeAPIServerURL string `yaml:"kube_api_server_url"`

	// KubeNamespace is the namespace of the portal Endpoints object.
	// Env: JAMSESH_ROUTER_KUBE_NAMESPACE
	KubeNamespace string `yaml:"kube_namespace"`

	// KubeServiceName is the name of the portal Endpoints object.
	// Env: JAMSESH_ROUTER_KUBE_SERVICE_NAME
	KubeServiceName string `yaml:"kube_service_name"`

	// KubePodPort is the port number appended to each discovered pod IP.
	// Env: JAMSESH_ROUTER_KUBE_POD_PORT
	KubePodPort int `yaml:"kube_pod_port"`

	// KubeBearerToken is the bearer token sent on k8s API requests.
	// Leave empty for unauthenticated access (e.g. in tests).
	// Env: JAMSESH_ROUTER_KUBE_BEARER_TOKEN
	KubeBearerToken string `yaml:"kube_bearer_token"`

	// KubeResyncIntervalS is the re-list fallback interval in seconds.
	// Default: 30. Env: JAMSESH_ROUTER_KUBE_RESYNC_INTERVAL_S
	KubeResyncIntervalS int `yaml:"kube_resync_interval_s"`

	// ProbeInterval is how often the discovery loop polls pod readiness.
	// Default: 5s. YAML-only in v1.
	ProbeInterval time.Duration `yaml:"probe_interval"`

	// ProbeTimeout is the per-pod /readyz HTTP timeout.
	// Default: 2s. YAML-only in v1.
	ProbeTimeout time.Duration `yaml:"probe_timeout"`

	// HintCacheTTL is the per-entry TTL for the soft-coordinator hint cache.
	// Default: 60s. YAML-only in v1.
	HintCacheTTL time.Duration `yaml:"hint_cache_ttl"`

	// Vnodes is the number of virtual nodes per real pod on the consistent-hash
	// ring. Higher values give more even distribution at the cost of memory.
	// Default: 150. YAML-only in v1.
	Vnodes int `yaml:"vnodes"`

	// ShutdownGraceSeconds is the wall-clock budget for graceful HTTP drain.
	// Default: 30. Env: JAMSESH_ROUTER_SHUTDOWN_GRACE_S
	ShutdownGraceSeconds int `yaml:"shutdown_grace_s"`
}

// Load reads configuration from an optional YAML file at path, then
// overlays environment variables. Returns validated defaults when path
// is empty and no env vars are set.
func Load(path string) (Config, error) {
	cfg := defaults()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("router config: read %s: %w", path, err)
		}
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("router config: parse %s: %w", path, err)
		}
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate checks invariants that cannot be expressed as YAML types.
// It is also called internally by Load.
func (c Config) Validate() error {
	if c.Bind == "" {
		return fmt.Errorf("router config: bind must not be empty")
	}
	switch c.DiscoveryMode {
	case DiscoveryStatic:
		if len(c.StaticPods) == 0 {
			return fmt.Errorf("router config: static_pods must not be empty")
		}
	case DiscoveryKubernetes:
		// KubeAPIServerURL is optional when KUBERNETES_SERVICE_HOST is set —
		// the in-cluster constructor auto-derives it from https://kubernetes.default.svc.
		// When neither is provided, K8sInCluster will fail loudly at wiring time.
		if c.KubeAPIServerURL == "" && os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
			return fmt.Errorf("router config: kube_api_server_url must not be empty in kubernetes discovery mode (or set KUBERNETES_SERVICE_HOST for in-cluster auto-detection)")
		}
		if c.KubeNamespace == "" {
			return fmt.Errorf("router config: kube_namespace must not be empty in kubernetes discovery mode")
		}
		if c.KubeServiceName == "" {
			return fmt.Errorf("router config: kube_service_name must not be empty in kubernetes discovery mode")
		}
		if c.KubePodPort <= 0 {
			return fmt.Errorf("router config: kube_pod_port must be a positive integer in kubernetes discovery mode, got %d", c.KubePodPort)
		}
	default:
		return fmt.Errorf("router config: unknown discovery_mode %q (must be %q or %q)", c.DiscoveryMode, DiscoveryStatic, DiscoveryKubernetes)
	}
	if c.Vnodes <= 0 {
		return fmt.Errorf("router config: vnodes must be a positive integer, got %d", c.Vnodes)
	}
	if c.ProbeInterval <= 0 {
		return fmt.Errorf("router config: probe_interval must be positive, got %v", c.ProbeInterval)
	}
	if c.ProbeTimeout <= 0 {
		return fmt.Errorf("router config: probe_timeout must be positive, got %v", c.ProbeTimeout)
	}
	if c.HintCacheTTL <= 0 {
		return fmt.Errorf("router config: hint_cache_ttl must be positive, got %v", c.HintCacheTTL)
	}
	if c.ShutdownGraceSeconds <= 0 {
		return fmt.Errorf("router config: shutdown_grace_s must be a positive integer, got %d", c.ShutdownGraceSeconds)
	}
	return nil
}

// defaults returns baseline configuration with sensible values.
func defaults() Config {
	return Config{
		Bind:                 ":8080",
		DiscoveryMode:        DiscoveryStatic,
		ProbeInterval:        5 * time.Second,
		ProbeTimeout:         2 * time.Second,
		HintCacheTTL:         60 * time.Second,
		Vnodes:               150,
		ShutdownGraceSeconds: 30,
		KubeResyncIntervalS:  30,
	}
}

// applyEnv overlays environment variables onto cfg.
// Only non-empty env values take effect; missing vars leave existing value.
func applyEnv(c *Config) {
	if v := os.Getenv("JAMSESH_ROUTER_BIND"); v != "" {
		c.Bind = v
	}
	if v := os.Getenv("JAMSESH_ROUTER_DISCOVERY_MODE"); v != "" {
		c.DiscoveryMode = DiscoveryMode(v)
	}
	if v := os.Getenv("JAMSESH_ROUTER_STATIC_PODS"); v != "" {
		// Comma-separated list; trim whitespace around each entry.
		parts := strings.Split(v, ",")
		pods := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				pods = append(pods, p)
			}
		}
		if len(pods) > 0 {
			c.StaticPods = pods
		}
	}
	if v := os.Getenv("JAMSESH_ROUTER_SHUTDOWN_GRACE_S"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.ShutdownGraceSeconds = n
		}
	}
	// Kubernetes discovery env vars.
	if v := os.Getenv("JAMSESH_ROUTER_KUBE_API_SERVER_URL"); v != "" {
		c.KubeAPIServerURL = v
	}
	if v := os.Getenv("JAMSESH_ROUTER_KUBE_NAMESPACE"); v != "" {
		c.KubeNamespace = v
	}
	if v := os.Getenv("JAMSESH_ROUTER_KUBE_SERVICE_NAME"); v != "" {
		c.KubeServiceName = v
	}
	if v := os.Getenv("JAMSESH_ROUTER_KUBE_POD_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.KubePodPort = n
		}
	}
	if v := os.Getenv("JAMSESH_ROUTER_KUBE_BEARER_TOKEN"); v != "" {
		c.KubeBearerToken = v
	}
	if v := os.Getenv("JAMSESH_ROUTER_KUBE_RESYNC_INTERVAL_S"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.KubeResyncIntervalS = n
		}
	}
}
