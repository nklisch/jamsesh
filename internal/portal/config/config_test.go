package config_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	if cfg.DB.MaxOpenConns != 25 {
		t.Errorf("DB.MaxOpenConns: got %d, want 25", cfg.DB.MaxOpenConns)
	}
	if cfg.DB.MaxIdleConns != 5 {
		t.Errorf("DB.MaxIdleConns: got %d, want 5", cfg.DB.MaxIdleConns)
	}
	if cfg.DB.ConnMaxLifetime != 30*time.Minute {
		t.Errorf("DB.ConnMaxLifetime: got %v, want 30m", cfg.DB.ConnMaxLifetime)
	}
	if cfg.ShutdownGraceSeconds != 30 {
		t.Errorf("ShutdownGraceSeconds default: got %d, want 30", cfg.ShutdownGraceSeconds)
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

// TestDBConfigDefaults verifies that DB pool defaults are applied when
// neither a YAML key nor an env var is set.
func TestDBConfigDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DB.MaxOpenConns != 25 {
		t.Errorf("DB.MaxOpenConns default: got %d, want 25", cfg.DB.MaxOpenConns)
	}
	if cfg.DB.MaxIdleConns != 5 {
		t.Errorf("DB.MaxIdleConns default: got %d, want 5", cfg.DB.MaxIdleConns)
	}
	if cfg.DB.ConnMaxLifetime != 30*time.Minute {
		t.Errorf("DB.ConnMaxLifetime default: got %v, want 30m", cfg.DB.ConnMaxLifetime)
	}
}

// TestDBConfigEnvOverride verifies that the three JAMSESH_DB_* pool env vars
// override the defaults.
func TestDBConfigEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DB_MAX_OPEN_CONNS", "50")
	t.Setenv("JAMSESH_DB_MAX_IDLE_CONNS", "10")
	t.Setenv("JAMSESH_DB_CONN_MAX_LIFETIME", "1h")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DB.MaxOpenConns != 50 {
		t.Errorf("DB.MaxOpenConns: got %d, want 50", cfg.DB.MaxOpenConns)
	}
	if cfg.DB.MaxIdleConns != 10 {
		t.Errorf("DB.MaxIdleConns: got %d, want 10", cfg.DB.MaxIdleConns)
	}
	if cfg.DB.ConnMaxLifetime != time.Hour {
		t.Errorf("DB.ConnMaxLifetime: got %v, want 1h", cfg.DB.ConnMaxLifetime)
	}
}

// TestDBConfigYAML verifies that the db.* YAML keys are parsed correctly.
func TestDBConfigYAML(t *testing.T) {
	clearEnv(t)
	yaml := `
db:
  max_open_conns: 100
  max_idle_conns: 20
  conn_max_lifetime: 45m
`
	path := writeTempConfig(t, yaml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DB.MaxOpenConns != 100 {
		t.Errorf("DB.MaxOpenConns: got %d, want 100", cfg.DB.MaxOpenConns)
	}
	if cfg.DB.MaxIdleConns != 20 {
		t.Errorf("DB.MaxIdleConns: got %d, want 20", cfg.DB.MaxIdleConns)
	}
	if cfg.DB.ConnMaxLifetime != 45*time.Minute {
		t.Errorf("DB.ConnMaxLifetime: got %v, want 45m", cfg.DB.ConnMaxLifetime)
	}
}

// TestShutdownGraceSecondsDefault verifies the default value is 30.
func TestShutdownGraceSecondsDefault(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ShutdownGraceSeconds != 30 {
		t.Errorf("ShutdownGraceSeconds default: got %d, want 30", cfg.ShutdownGraceSeconds)
	}
}

// TestShutdownGraceSecondsEnvOverride verifies JAMSESH_SHUTDOWN_GRACE_S is applied.
func TestShutdownGraceSecondsEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_SHUTDOWN_GRACE_S", "60")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ShutdownGraceSeconds != 60 {
		t.Errorf("ShutdownGraceSeconds: got %d, want 60", cfg.ShutdownGraceSeconds)
	}
}

// TestShutdownGraceSecondsYAML verifies the shutdown_grace_s YAML key is parsed.
func TestShutdownGraceSecondsYAML(t *testing.T) {
	clearEnv(t)
	yamlContent := "shutdown_grace_s: 45\n"
	path := writeTempConfig(t, yamlContent)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ShutdownGraceSeconds != 45 {
		t.Errorf("ShutdownGraceSeconds: got %d, want 45", cfg.ShutdownGraceSeconds)
	}
}

// TestShutdownGraceSecondsValidation verifies that zero and negative values are rejected.
func TestShutdownGraceSecondsValidation(t *testing.T) {
	clearEnv(t)

	for _, bad := range []string{"0", "-1", "-30"} {
		t.Run("grace_s="+bad, func(t *testing.T) {
			t.Setenv("JAMSESH_SHUTDOWN_GRACE_S", bad)
			_, err := config.Load("")
			if err == nil {
				t.Errorf("expected validation error for JAMSESH_SHUTDOWN_GRACE_S=%s, got nil", bad)
			}
		})
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
		"JAMSESH_OAUTH_GITHUB_CLIENT_ID", "JAMSESH_OAUTH_GITHUB_CLIENT_SECRET",
		"JAMSESH_OAUTH_GITHUB_BASE_URL",
		"JAMSESH_DB_MAX_OPEN_CONNS", "JAMSESH_DB_MAX_IDLE_CONNS",
		"JAMSESH_DB_CONN_MAX_LIFETIME",
		"JAMSESH_SHUTDOWN_GRACE_S",
		"JAMSESH_DEPLOY_MODE",
		"JAMSESH_LEASE_HEARTBEAT_INTERVAL_S",
		"JAMSESH_LEASE_RETENTION_DAYS",
		"JAMSESH_LEASE_RETENTION_INTERVAL_HOURS",
	}
	for _, v := range vars {
		t.Setenv(v, "") // t.Setenv restores on cleanup; set to "" to clear
		os.Unsetenv(v)  //nolint:errcheck
	}
}

// TestOAuthGitHubBaseURLEnvOverride verifies that JAMSESH_OAUTH_GITHUB_BASE_URL
// flows through config.Load into cfg.OAuth.GitHub.BaseURL.
func TestOAuthGitHubBaseURLEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_OAUTH_GITHUB_BASE_URL", "https://fake.example.com")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.OAuth.GitHub.BaseURL, "https://fake.example.com"; got != want {
		t.Errorf("BaseURL: got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Lease config tests
// ---------------------------------------------------------------------------

// TestLeaseConfigDefaults verifies the default values for all JAMSESH_LEASE_*
// and JAMSESH_DEPLOY_MODE fields.
func TestLeaseConfigDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DeployMode != "single" {
		t.Errorf("DeployMode default: got %q, want %q", cfg.DeployMode, "single")
	}
	if cfg.LeaseHeartbeatIntervalS != 10 {
		t.Errorf("LeaseHeartbeatIntervalS default: got %d, want 10", cfg.LeaseHeartbeatIntervalS)
	}
	if cfg.LeaseRetentionDays != 30 {
		t.Errorf("LeaseRetentionDays default: got %d, want 30", cfg.LeaseRetentionDays)
	}
	if cfg.LeaseRetentionIntervalHours != 1 {
		t.Errorf("LeaseRetentionIntervalHours default: got %d, want 1", cfg.LeaseRetentionIntervalHours)
	}
}

// TestLeaseConfigEnvOverride verifies that the JAMSESH_LEASE_* and
// JAMSESH_DEPLOY_MODE env vars are applied correctly.
func TestLeaseConfigEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DEPLOY_MODE", "clustered")
	t.Setenv("JAMSESH_DB_DRIVER", "postgres") // clustered requires postgres
	t.Setenv("JAMSESH_LEASE_HEARTBEAT_INTERVAL_S", "30")
	t.Setenv("JAMSESH_LEASE_RETENTION_DAYS", "90")
	t.Setenv("JAMSESH_LEASE_RETENTION_INTERVAL_HOURS", "6")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DeployMode != "clustered" {
		t.Errorf("DeployMode: got %q, want %q", cfg.DeployMode, "clustered")
	}
	if cfg.LeaseHeartbeatIntervalS != 30 {
		t.Errorf("LeaseHeartbeatIntervalS: got %d, want 30", cfg.LeaseHeartbeatIntervalS)
	}
	if cfg.LeaseRetentionDays != 90 {
		t.Errorf("LeaseRetentionDays: got %d, want 90", cfg.LeaseRetentionDays)
	}
	if cfg.LeaseRetentionIntervalHours != 6 {
		t.Errorf("LeaseRetentionIntervalHours: got %d, want 6", cfg.LeaseRetentionIntervalHours)
	}
}

// TestValidation_DeployModeSingle verifies that deploy_mode=single is valid.
func TestValidation_DeployModeSingle(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DEPLOY_MODE", "single")
	if _, err := config.Load(""); err != nil {
		t.Errorf("deploy_mode=single should be valid: %v", err)
	}
}

// TestValidation_DeployModeClustered verifies that deploy_mode=clustered with
// db_driver=postgres is valid.
func TestValidation_DeployModeClustered(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DEPLOY_MODE", "clustered")
	t.Setenv("JAMSESH_DB_DRIVER", "postgres")
	if _, err := config.Load(""); err != nil {
		t.Errorf("deploy_mode=clustered with db_driver=postgres should be valid: %v", err)
	}
}

// TestValidation_DeployModeInvalid verifies that an unknown deploy_mode is rejected.
func TestValidation_DeployModeInvalid(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DEPLOY_MODE", "distributed")
	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected error for unknown deploy_mode, got nil")
	}
}

// TestValidation_ClusteredWithSQLite verifies that deploy_mode=clustered AND
// db_driver=sqlite is rejected at validation time.
func TestValidation_ClusteredWithSQLite(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DEPLOY_MODE", "clustered")
	t.Setenv("JAMSESH_DB_DRIVER", "sqlite")
	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected error for deploy_mode=clustered + db_driver=sqlite, got nil")
	}
}

// TestValidation_LeaseIntervalNonPositive verifies that zero/negative lease
// interval values are rejected.
func TestValidation_LeaseIntervalNonPositive(t *testing.T) {
	clearEnv(t)

	t.Run("heartbeat_interval_s=0", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("JAMSESH_LEASE_HEARTBEAT_INTERVAL_S", "0")
		_, err := config.Load("")
		if err == nil {
			t.Error("expected error for JAMSESH_LEASE_HEARTBEAT_INTERVAL_S=0, got nil")
		}
	})

	t.Run("retention_days=0", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("JAMSESH_LEASE_RETENTION_DAYS", "0")
		_, err := config.Load("")
		if err == nil {
			t.Error("expected error for JAMSESH_LEASE_RETENTION_DAYS=0, got nil")
		}
	})

	t.Run("retention_interval_hours=0", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("JAMSESH_LEASE_RETENTION_INTERVAL_HOURS", "0")
		_, err := config.Load("")
		if err == nil {
			t.Error("expected error for JAMSESH_LEASE_RETENTION_INTERVAL_HOURS=0, got nil")
		}
	})
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
