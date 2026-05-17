package config_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"jamsesh/internal/portal/config"
)

func TestDefaults(t *testing.T) {
	// Ensure no JAMSESH_ vars leak in from the test environment.
	clearEnv(t)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error: %v", err)
	}
	if cfg.Bind != ":8443" {
		t.Errorf("Bind: got %q, want %q", cfg.Bind, ":8443")
	}
	if cfg.DBDriver != "sqlite" {
		t.Errorf("DBDriver: got %q, want %q", cfg.DBDriver, "sqlite")
	}
	if cfg.DBDSN != "./jamsesh.db" {
		t.Errorf("DBDSN: got %q, want %q", cfg.DBDSN, "./jamsesh.db")
	}
	if cfg.TLS.Mode != "behind_proxy" {
		t.Errorf("TLS.Mode: got %q, want %q", cfg.TLS.Mode, "behind_proxy")
	}
	if cfg.TLS.CertPath != "" {
		t.Errorf("TLS.CertPath: got %q, want empty", cfg.TLS.CertPath)
	}
	if cfg.TLS.KeyPath != "" {
		t.Errorf("TLS.KeyPath: got %q, want empty", cfg.TLS.KeyPath)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format: got %q, want %q", cfg.Log.Format, "json")
	}
	if cfg.Log.Level != slog.LevelInfo {
		t.Errorf("Log.Level: got %v, want %v", cfg.Log.Level, slog.LevelInfo)
	}
	if cfg.Storage != "./storage" {
		t.Errorf("Storage: got %q, want %q", cfg.Storage, "./storage")
	}
	const wantMaxPack = int64(52428800)
	if cfg.Git.MaxPackBytes != wantMaxPack {
		t.Errorf("Git.MaxPackBytes: got %d, want %d", cfg.Git.MaxPackBytes, wantMaxPack)
	}
}

func TestGitMaxPackBytesEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_GIT_MAX_PACK_BYTES", "104857600") // 100 MiB

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Git.MaxPackBytes != 104857600 {
		t.Errorf("Git.MaxPackBytes: got %d, want 104857600", cfg.Git.MaxPackBytes)
	}
}

func TestGitMaxPackBytesYAML(t *testing.T) {
	clearEnv(t)
	yaml := `
git:
  max_pack_bytes: 10485760
`
	path := writeTempConfig(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Git.MaxPackBytes != 10485760 {
		t.Errorf("Git.MaxPackBytes: got %d, want 10485760", cfg.Git.MaxPackBytes)
	}
}

func TestYAMLLoad(t *testing.T) {
	clearEnv(t)

	yaml := `
bind: "127.0.0.1:9000"
db_driver: postgres
db_dsn: "postgres://localhost/jamsesh"
tls:
  mode: native
  cert_path: /etc/certs/cert.pem
  key_path: /etc/certs/key.pem
log:
  format: text
  level: -4
storage: /var/jamsesh/storage
`
	path := writeTempConfig(t, yaml)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load(yaml) error: %v", err)
	}
	if cfg.Bind != "127.0.0.1:9000" {
		t.Errorf("Bind: got %q", cfg.Bind)
	}
	if cfg.DBDriver != "postgres" {
		t.Errorf("DBDriver: got %q", cfg.DBDriver)
	}
	if cfg.DBDSN != "postgres://localhost/jamsesh" {
		t.Errorf("DBDSN: got %q", cfg.DBDSN)
	}
	if cfg.TLS.Mode != "native" {
		t.Errorf("TLS.Mode: got %q", cfg.TLS.Mode)
	}
	if cfg.TLS.CertPath != "/etc/certs/cert.pem" {
		t.Errorf("TLS.CertPath: got %q", cfg.TLS.CertPath)
	}
	if cfg.TLS.KeyPath != "/etc/certs/key.pem" {
		t.Errorf("TLS.KeyPath: got %q", cfg.TLS.KeyPath)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format: got %q", cfg.Log.Format)
	}
	if cfg.Log.Level != slog.LevelDebug {
		t.Errorf("Log.Level: got %v, want DEBUG (-4)", cfg.Log.Level)
	}
	if cfg.Storage != "/var/jamsesh/storage" {
		t.Errorf("Storage: got %q", cfg.Storage)
	}
}

func TestEnvOverride(t *testing.T) {
	clearEnv(t)

	// Set a base config via YAML then override every field with env vars.
	yaml := `
bind: ":9000"
db_driver: sqlite
db_dsn: ./base.db
tls:
  mode: behind_proxy
log:
  format: json
  level: 0
storage: ./base-storage
`
	path := writeTempConfig(t, yaml)

	t.Setenv("JAMSESH_BIND", "0.0.0.0:7777")
	t.Setenv("JAMSESH_DB_DRIVER", "postgres")
	t.Setenv("JAMSESH_DB_DSN", "postgres://override/db")
	t.Setenv("JAMSESH_TLS_MODE", "behind_proxy") // keep valid
	t.Setenv("JAMSESH_LOG_FORMAT", "text")
	t.Setenv("JAMSESH_LOG_LEVEL", "-4")
	t.Setenv("JAMSESH_STORAGE", "/tmp/override-storage")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Bind != "0.0.0.0:7777" {
		t.Errorf("Bind: got %q, want %q", cfg.Bind, "0.0.0.0:7777")
	}
	if cfg.DBDriver != "postgres" {
		t.Errorf("DBDriver: got %q, want postgres", cfg.DBDriver)
	}
	if cfg.DBDSN != "postgres://override/db" {
		t.Errorf("DBDSN: got %q", cfg.DBDSN)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format: got %q, want text", cfg.Log.Format)
	}
	if cfg.Log.Level != slog.LevelDebug {
		t.Errorf("Log.Level: got %v, want DEBUG", cfg.Log.Level)
	}
	if cfg.Storage != "/tmp/override-storage" {
		t.Errorf("Storage: got %q", cfg.Storage)
	}
}

func TestEnvOverrideWithoutFile(t *testing.T) {
	clearEnv(t)

	t.Setenv("JAMSESH_BIND", "127.0.0.1:5000")
	t.Setenv("JAMSESH_TLS_MODE", "behind_proxy")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Bind != "127.0.0.1:5000" {
		t.Errorf("Bind: got %q, want 127.0.0.1:5000", cfg.Bind)
	}
	// Defaults preserved for non-overridden fields.
	if cfg.DBDriver != "sqlite" {
		t.Errorf("DBDriver: got %q, want sqlite (default)", cfg.DBDriver)
	}
}

func TestValidation_NativeModeMissingCerts(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_TLS_MODE", "native")
	// No cert/key env vars — must fail.
	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected error for tls.mode=native without cert/key, got nil")
	}
}

func TestValidation_NativeModeWithCerts(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_TLS_MODE", "native")
	t.Setenv("JAMSESH_TLS_CERT", "/path/to/cert.pem")
	t.Setenv("JAMSESH_TLS_KEY", "/path/to/key.pem")
	_, err := config.Load("")
	if err != nil {
		t.Fatalf("expected success for valid native TLS config, got: %v", err)
	}
}

func TestValidation_BehindProxyNoCerts(t *testing.T) {
	clearEnv(t)
	// Default mode is behind_proxy — should succeed without cert material.
	_, err := config.Load("")
	if err != nil {
		t.Fatalf("expected success for behind_proxy mode, got: %v", err)
	}
}

func TestValidation_InvalidTLSMode(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_TLS_MODE", "terminator")
	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected error for invalid tls.mode, got nil")
	}
}

func TestValidation_InvalidDBDriver(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DB_DRIVER", "mysql")
	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected error for invalid db_driver, got nil")
	}
}

func TestValidation_SQLiteDriver(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DB_DRIVER", "sqlite")
	_, err := config.Load("")
	if err != nil {
		t.Fatalf("sqlite driver should be valid: %v", err)
	}
}

func TestValidation_PostgresDriver(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DB_DRIVER", "postgres")
	_, err := config.Load("")
	if err != nil {
		t.Fatalf("postgres driver should be valid: %v", err)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	clearEnv(t)
	_, err := config.Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	clearEnv(t)
	path := writeTempConfig(t, "bind: [invalid yaml\n")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// clearEnv unsets all JAMSESH_ environment variables for test isolation.
func clearEnv(t *testing.T) {
	t.Helper()
	vars := []string{
		"JAMSESH_BIND", "JAMSESH_DB_DRIVER", "JAMSESH_DB_DSN",
		"JAMSESH_TLS_MODE", "JAMSESH_TLS_CERT", "JAMSESH_TLS_KEY",
		"JAMSESH_LOG_FORMAT", "JAMSESH_LOG_LEVEL", "JAMSESH_STORAGE",
		"JAMSESH_GIT_MAX_PACK_BYTES",
	}
	for _, v := range vars {
		t.Setenv(v, "") // t.Setenv restores on cleanup; set to "" to clear
		os.Unsetenv(v)  //nolint:errcheck
	}
}

// writeTempConfig writes content to a temp YAML file and returns its path.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeTempConfig: %v", err)
	}
	return path
}
