// Package config loads and validates the jamsesh-router configuration.
// Config is sourced from an optional YAML file with env-var overrides.
//
// YAML keys: bind, discovery_mode, static_pods, kube_namespace,
//
//	kube_service_name, probe_interval, probe_timeout, hint_cache_ttl,
//	vnodes, shutdown_grace_s
//
// Env vars:
//
//	JAMSESH_ROUTER_BIND               – listen address, default ":8080"
//	JAMSESH_ROUTER_DISCOVERY_MODE     – "static" | "kubernetes"
//	JAMSESH_ROUTER_STATIC_PODS        – comma-separated "host:port" list
//	JAMSESH_ROUTER_KUBE_NAMESPACE     – k8s namespace (kubernetes mode)
//	JAMSESH_ROUTER_KUBE_SERVICE_NAME  – k8s service name (kubernetes mode)
//	JAMSESH_ROUTER_SHUTDOWN_GRACE_S   – seconds for graceful drain
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

// Config holds the full jamsesh-router configuration.
type Config struct {
	// Bind is the TCP address the router listens on. Default: ":8080".
	// Env: JAMSESH_ROUTER_BIND
	Bind string `yaml:"bind"`

	// DiscoveryMode controls how the pod set is populated.
	// "static"     — use StaticPods list directly.
	// "kubernetes" — watch the k8s service (Unit 3, discovery story).
	// Env: JAMSESH_ROUTER_DISCOVERY_MODE
	DiscoveryMode string `yaml:"discovery_mode"`

	// StaticPods is the fixed list of pod addresses ("host:port") used when
	// DiscoveryMode is "static". Required when DiscoveryMode == "static".
	// Env: JAMSESH_ROUTER_STATIC_PODS (comma-separated)
	StaticPods []string `yaml:"static_pods"`

	// KubeNamespace is the k8s namespace containing the portal pods.
	// Required when DiscoveryMode == "kubernetes".
	// Env: JAMSESH_ROUTER_KUBE_NAMESPACE
	KubeNamespace string `yaml:"kube_namespace"`

	// KubeServiceName is the k8s service name whose endpoints back the portal.
	// Required when DiscoveryMode == "kubernetes".
	// Env: JAMSESH_ROUTER_KUBE_SERVICE_NAME
	KubeServiceName string `yaml:"kube_service_name"`

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
	case "static":
		if len(c.StaticPods) == 0 {
			return fmt.Errorf("router config: static_pods must not be empty when discovery_mode is \"static\"")
		}
	case "kubernetes":
		// KubeNamespace and KubeServiceName are required for k8s mode.
		// The discovery story (Unit 3) enforces these at Discoverer construction;
		// we accept blank here so Unit 2 can be tested without k8s.
	default:
		return fmt.Errorf("router config: discovery_mode must be \"static\" or \"kubernetes\", got %q", c.DiscoveryMode)
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
		DiscoveryMode:        "static",
		ProbeInterval:        5 * time.Second,
		ProbeTimeout:         2 * time.Second,
		HintCacheTTL:         60 * time.Second,
		Vnodes:               150,
		ShutdownGraceSeconds: 30,
	}
}

// applyEnv overlays environment variables onto cfg.
// Only non-empty env values take effect; missing vars leave existing value.
func applyEnv(c *Config) {
	if v := os.Getenv("JAMSESH_ROUTER_BIND"); v != "" {
		c.Bind = v
	}
	if v := os.Getenv("JAMSESH_ROUTER_DISCOVERY_MODE"); v != "" {
		c.DiscoveryMode = v
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
	if v := os.Getenv("JAMSESH_ROUTER_KUBE_NAMESPACE"); v != "" {
		c.KubeNamespace = v
	}
	if v := os.Getenv("JAMSESH_ROUTER_KUBE_SERVICE_NAME"); v != "" {
		c.KubeServiceName = v
	}
	if v := os.Getenv("JAMSESH_ROUTER_SHUTDOWN_GRACE_S"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.ShutdownGraceSeconds = n
		}
	}
}
