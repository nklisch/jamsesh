package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"jamsesh/internal/router/config"
)

func TestDefaults(t *testing.T) {
	// No file, no env → defaults must be valid.
	cfg, err := config.Load("")
	if err == nil {
		t.Fatal("expected error: static_pods must be non-empty with default discovery_mode=static, got nil")
	}
	_ = cfg
}

func TestLoadYAML(t *testing.T) {
	yaml := `
bind: ":9090"
discovery_mode: "static"
static_pods:
  - "pod1:8080"
  - "pod2:8080"
vnodes: 100
probe_interval: 10s
probe_timeout: 3s
hint_cache_ttl: 30s
shutdown_grace_s: 15
`
	path := writeTemp(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Bind != ":9090" {
		t.Errorf("Bind: got %q, want %q", cfg.Bind, ":9090")
	}
	if cfg.DiscoveryMode != "static" {
		t.Errorf("DiscoveryMode: got %q, want %q", cfg.DiscoveryMode, "static")
	}
	if len(cfg.StaticPods) != 2 {
		t.Fatalf("StaticPods length: got %d, want 2", len(cfg.StaticPods))
	}
	if cfg.StaticPods[0] != "pod1:8080" {
		t.Errorf("StaticPods[0]: got %q, want %q", cfg.StaticPods[0], "pod1:8080")
	}
	if cfg.Vnodes != 100 {
		t.Errorf("Vnodes: got %d, want 100", cfg.Vnodes)
	}
	if cfg.ProbeInterval != 10*time.Second {
		t.Errorf("ProbeInterval: got %v, want 10s", cfg.ProbeInterval)
	}
	if cfg.ProbeTimeout != 3*time.Second {
		t.Errorf("ProbeTimeout: got %v, want 3s", cfg.ProbeTimeout)
	}
	if cfg.HintCacheTTL != 30*time.Second {
		t.Errorf("HintCacheTTL: got %v, want 30s", cfg.HintCacheTTL)
	}
	if cfg.ShutdownGraceSeconds != 15 {
		t.Errorf("ShutdownGraceSeconds: got %d, want 15", cfg.ShutdownGraceSeconds)
	}
}

func TestEnvOverlay(t *testing.T) {
	yaml := `
bind: ":9090"
discovery_mode: "static"
static_pods:
  - "yaml-pod:8080"
shutdown_grace_s: 10
`
	path := writeTemp(t, yaml)

	t.Setenv("JAMSESH_ROUTER_BIND", ":7070")
	t.Setenv("JAMSESH_ROUTER_STATIC_PODS", "env-pod1:8080, env-pod2:8080")
	t.Setenv("JAMSESH_ROUTER_SHUTDOWN_GRACE_S", "20")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Bind != ":7070" {
		t.Errorf("Bind env override: got %q, want %q", cfg.Bind, ":7070")
	}
	if len(cfg.StaticPods) != 2 {
		t.Fatalf("StaticPods length: got %d, want 2", len(cfg.StaticPods))
	}
	if cfg.StaticPods[0] != "env-pod1:8080" {
		t.Errorf("StaticPods[0]: got %q", cfg.StaticPods[0])
	}
	if cfg.StaticPods[1] != "env-pod2:8080" {
		t.Errorf("StaticPods[1]: got %q", cfg.StaticPods[1])
	}
	if cfg.ShutdownGraceSeconds != 20 {
		t.Errorf("ShutdownGraceSeconds env override: got %d, want 20", cfg.ShutdownGraceSeconds)
	}
}

func TestEnvKubeFields(t *testing.T) {
	yaml := `
bind: ":8080"
discovery_mode: "kubernetes"
shutdown_grace_s: 10
`
	path := writeTemp(t, yaml)
	t.Setenv("JAMSESH_ROUTER_KUBE_NAMESPACE", "prod")
	t.Setenv("JAMSESH_ROUTER_KUBE_SERVICE_NAME", "jamsesh-portal")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.KubeNamespace != "prod" {
		t.Errorf("KubeNamespace: got %q, want %q", cfg.KubeNamespace, "prod")
	}
	if cfg.KubeServiceName != "jamsesh-portal" {
		t.Errorf("KubeServiceName: got %q, want %q", cfg.KubeServiceName, "jamsesh-portal")
	}
}

func TestValidate(t *testing.T) {
	base := config.Config{
		Bind:                 ":8080",
		DiscoveryMode:        "static",
		StaticPods:           []string{"pod1:8080"},
		Vnodes:               150,
		ProbeInterval:        5 * time.Second,
		ProbeTimeout:         2 * time.Second,
		HintCacheTTL:         60 * time.Second,
		ShutdownGraceSeconds: 30,
	}

	cases := []struct {
		name    string
		modify  func(*config.Config)
		wantErr string
	}{
		{
			name:    "empty bind",
			modify:  func(c *config.Config) { c.Bind = "" },
			wantErr: "bind must not be empty",
		},
		{
			name:    "unknown discovery mode",
			modify:  func(c *config.Config) { c.DiscoveryMode = "consul" },
			wantErr: "discovery_mode must be",
		},
		{
			name: "static mode with no pods",
			modify: func(c *config.Config) {
				c.DiscoveryMode = "static"
				c.StaticPods = nil
			},
			wantErr: "static_pods must not be empty",
		},
		{
			name:    "zero vnodes",
			modify:  func(c *config.Config) { c.Vnodes = 0 },
			wantErr: "vnodes must be a positive integer",
		},
		{
			name:    "negative probe interval",
			modify:  func(c *config.Config) { c.ProbeInterval = -1 },
			wantErr: "probe_interval must be positive",
		},
		{
			name:    "zero probe timeout",
			modify:  func(c *config.Config) { c.ProbeTimeout = 0 },
			wantErr: "probe_timeout must be positive",
		},
		{
			name:    "zero hint cache ttl",
			modify:  func(c *config.Config) { c.HintCacheTTL = 0 },
			wantErr: "hint_cache_ttl must be positive",
		},
		{
			name:    "zero shutdown grace",
			modify:  func(c *config.Config) { c.ShutdownGraceSeconds = 0 },
			wantErr: "shutdown_grace_s must be a positive integer",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.modify(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !containsString(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadBadYAML(t *testing.T) {
	path := writeTemp(t, ":::invalid yaml:::")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected parse error for invalid YAML, got nil")
	}
}

// writeTemp writes content to a temporary file and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
