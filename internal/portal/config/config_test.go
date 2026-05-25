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
		"JAMSESH_OBJECT_STORAGE_URL",
		"JAMSESH_OBJECT_STORAGE_REGION",
		"JAMSESH_OBJECT_STORAGE_ENDPOINT_URL",
		"JAMSESH_OBJECT_STORAGE_PATH_STYLE",
		"JAMSESH_OBJECT_STORAGE_SYNC_QUEUE_SIZE",
		"JAMSESH_HYDRATION_IDLE_TIMEOUT_S",
		"JAMSESH_HYDRATION_CACHE_MAX_BYTES",
		"JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S",
		"JAMSESH_HYDRATION_WORKERS",
		"JAMSESH_PLAYGROUND_ENABLED",
		"JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S",
		"JAMSESH_PLAYGROUND_HARD_CAP_S",
		"JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR",
		"JAMSESH_PLAYGROUND_MAX_PARTICIPANTS",
		"JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES",
		"JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S",
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
	t.Setenv("JAMSESH_DB_DRIVER", "postgres")                       // clustered requires postgres
	t.Setenv("JAMSESH_OBJECT_STORAGE_URL", "s3://bucket/prefix")    // clustered requires object storage
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
// db_driver=postgres and object_storage_url is valid.
func TestValidation_DeployModeClustered(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DEPLOY_MODE", "clustered")
	t.Setenv("JAMSESH_DB_DRIVER", "postgres")
	t.Setenv("JAMSESH_OBJECT_STORAGE_URL", "s3://my-bucket/prefix")
	if _, err := config.Load(""); err != nil {
		t.Errorf("deploy_mode=clustered with db_driver=postgres and object_storage_url should be valid: %v", err)
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

// ---------------------------------------------------------------------------
// Object-storage config tests
// ---------------------------------------------------------------------------

// TestObjectStorageDefaults verifies the default values for object-storage fields.
func TestObjectStorageDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ObjectStorageURL != "" {
		t.Errorf("ObjectStorageURL default: got %q, want empty", cfg.ObjectStorageURL)
	}
	if cfg.ObjectStorageRegion != "" {
		t.Errorf("ObjectStorageRegion default: got %q, want empty", cfg.ObjectStorageRegion)
	}
	if cfg.ObjectStorageEndpointURL != "" {
		t.Errorf("ObjectStorageEndpointURL default: got %q, want empty", cfg.ObjectStorageEndpointURL)
	}
	if cfg.ObjectStoragePathStyle {
		t.Errorf("ObjectStoragePathStyle default: got true, want false")
	}
	if cfg.ObjectStorageSyncQueueSize != 256 {
		t.Errorf("ObjectStorageSyncQueueSize default: got %d, want 256", cfg.ObjectStorageSyncQueueSize)
	}
}

// TestObjectStorageEnvOverride verifies that all five JAMSESH_OBJECT_STORAGE_*
// env vars are applied correctly.
func TestObjectStorageEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_OBJECT_STORAGE_URL", "s3://my-bucket/jamsesh")
	t.Setenv("JAMSESH_OBJECT_STORAGE_REGION", "eu-west-1")
	t.Setenv("JAMSESH_OBJECT_STORAGE_ENDPOINT_URL", "https://example.r2.cloudflarestorage.com")
	t.Setenv("JAMSESH_OBJECT_STORAGE_PATH_STYLE", "true")
	t.Setenv("JAMSESH_OBJECT_STORAGE_SYNC_QUEUE_SIZE", "512")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ObjectStorageURL != "s3://my-bucket/jamsesh" {
		t.Errorf("ObjectStorageURL: got %q, want %q", cfg.ObjectStorageURL, "s3://my-bucket/jamsesh")
	}
	if cfg.ObjectStorageRegion != "eu-west-1" {
		t.Errorf("ObjectStorageRegion: got %q, want eu-west-1", cfg.ObjectStorageRegion)
	}
	if cfg.ObjectStorageEndpointURL != "https://example.r2.cloudflarestorage.com" {
		t.Errorf("ObjectStorageEndpointURL: got %q", cfg.ObjectStorageEndpointURL)
	}
	if !cfg.ObjectStoragePathStyle {
		t.Errorf("ObjectStoragePathStyle: got false, want true")
	}
	if cfg.ObjectStorageSyncQueueSize != 512 {
		t.Errorf("ObjectStorageSyncQueueSize: got %d, want 512", cfg.ObjectStorageSyncQueueSize)
	}
}

// TestObjectStoragePathStyleFalse verifies that JAMSESH_OBJECT_STORAGE_PATH_STYLE=false
// is correctly parsed.
func TestObjectStoragePathStyleFalse(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_OBJECT_STORAGE_PATH_STYLE", "false")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ObjectStoragePathStyle {
		t.Errorf("ObjectStoragePathStyle: got true, want false for JAMSESH_OBJECT_STORAGE_PATH_STYLE=false")
	}
}

// TestObjectStorageYAML verifies that the object_storage_* YAML keys are parsed.
func TestObjectStorageYAML(t *testing.T) {
	clearEnv(t)
	yamlContent := `
object_storage_url: "gs://my-bucket/sessions"
object_storage_region: "us-central1"
object_storage_endpoint_url: ""
object_storage_path_style: false
object_storage_sync_queue_size: 128
`
	path := writeTempConfig(t, yamlContent)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ObjectStorageURL != "gs://my-bucket/sessions" {
		t.Errorf("ObjectStorageURL: got %q", cfg.ObjectStorageURL)
	}
	if cfg.ObjectStorageSyncQueueSize != 128 {
		t.Errorf("ObjectStorageSyncQueueSize: got %d, want 128", cfg.ObjectStorageSyncQueueSize)
	}
}

// TestValidation_ClusteredRequiresObjectStorage verifies that deploy_mode=clustered
// without object_storage_url is rejected at startup.
func TestValidation_ClusteredRequiresObjectStorage(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DEPLOY_MODE", "clustered")
	t.Setenv("JAMSESH_DB_DRIVER", "postgres")
	// No JAMSESH_OBJECT_STORAGE_URL — must fail.

	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected error for deploy_mode=clustered without object_storage_url, got nil")
	}
}

// TestValidation_SingleModeNoObjectStorage verifies that single-instance mode
// with no object_storage_url is valid (object storage is clustered-only).
func TestValidation_SingleModeNoObjectStorage(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DEPLOY_MODE", "single")
	// No object storage URL — should be fine in single mode.

	_, err := config.Load("")
	if err != nil {
		t.Errorf("single mode without object_storage_url should be valid: %v", err)
	}
}

// TestValidation_SingleModeWithObjectStorage verifies that single-instance mode
// with an object_storage_url set is accepted (the URL is ignored but not rejected).
func TestValidation_SingleModeWithObjectStorage(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_DEPLOY_MODE", "single")
	t.Setenv("JAMSESH_OBJECT_STORAGE_URL", "s3://bucket/prefix")

	_, err := config.Load("")
	if err != nil {
		t.Errorf("single mode with object_storage_url should be valid: %v", err)
	}
}

// TestValidation_ObjectStorageSyncQueueSizeZero verifies that a sync queue
// size of zero is rejected.
func TestValidation_ObjectStorageSyncQueueSizeZero(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_OBJECT_STORAGE_SYNC_QUEUE_SIZE", "0")

	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected error for object_storage_sync_queue_size=0, got nil")
	}
}

// TestValidation_ObjectStorageSyncQueueSizeNegative verifies that a negative
// sync queue size is rejected.
func TestValidation_ObjectStorageSyncQueueSizeNegative(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_OBJECT_STORAGE_SYNC_QUEUE_SIZE", "-1")

	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected error for object_storage_sync_queue_size=-1, got nil")
	}
}

// ---------------------------------------------------------------------------
// Hydration config tests
// ---------------------------------------------------------------------------

// TestHydrationConfigDefaults verifies the default values for hydration fields.
func TestHydrationConfigDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HydrationIdleTimeoutS != 300 {
		t.Errorf("HydrationIdleTimeoutS default: got %d, want 300", cfg.HydrationIdleTimeoutS)
	}
	if cfg.HydrationCacheMaxBytes != 0 {
		t.Errorf("HydrationCacheMaxBytes default: got %d, want 0 (unlimited)", cfg.HydrationCacheMaxBytes)
	}
	if cfg.HydrationIdleCheckPeriodS != 30 {
		t.Errorf("HydrationIdleCheckPeriodS default: got %d, want 30", cfg.HydrationIdleCheckPeriodS)
	}
	if cfg.HydrationWorkers != 8 {
		t.Errorf("HydrationWorkers default: got %d, want 8", cfg.HydrationWorkers)
	}
}

// TestHydrationConfigEnvOverride verifies that all four JAMSESH_HYDRATION_* env
// vars override the defaults correctly.
func TestHydrationConfigEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_HYDRATION_IDLE_TIMEOUT_S", "600")
	t.Setenv("JAMSESH_HYDRATION_CACHE_MAX_BYTES", "5368709120") // 5 GiB
	t.Setenv("JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S", "60")
	t.Setenv("JAMSESH_HYDRATION_WORKERS", "16")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HydrationIdleTimeoutS != 600 {
		t.Errorf("HydrationIdleTimeoutS: got %d, want 600", cfg.HydrationIdleTimeoutS)
	}
	if cfg.HydrationCacheMaxBytes != 5368709120 {
		t.Errorf("HydrationCacheMaxBytes: got %d, want 5368709120", cfg.HydrationCacheMaxBytes)
	}
	if cfg.HydrationIdleCheckPeriodS != 60 {
		t.Errorf("HydrationIdleCheckPeriodS: got %d, want 60", cfg.HydrationIdleCheckPeriodS)
	}
	if cfg.HydrationWorkers != 16 {
		t.Errorf("HydrationWorkers: got %d, want 16", cfg.HydrationWorkers)
	}
}

// TestHydrationConfigYAML verifies the hydration_* YAML keys are parsed.
func TestHydrationConfigYAML(t *testing.T) {
	clearEnv(t)
	yamlContent := `
hydration_idle_timeout_s: 120
hydration_cache_max_bytes: 1073741824
hydration_idle_check_period_s: 15
hydration_workers: 4
`
	path := writeTempConfig(t, yamlContent)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HydrationIdleTimeoutS != 120 {
		t.Errorf("HydrationIdleTimeoutS: got %d, want 120", cfg.HydrationIdleTimeoutS)
	}
	if cfg.HydrationCacheMaxBytes != 1073741824 {
		t.Errorf("HydrationCacheMaxBytes: got %d, want 1073741824", cfg.HydrationCacheMaxBytes)
	}
	if cfg.HydrationIdleCheckPeriodS != 15 {
		t.Errorf("HydrationIdleCheckPeriodS: got %d, want 15", cfg.HydrationIdleCheckPeriodS)
	}
	if cfg.HydrationWorkers != 4 {
		t.Errorf("HydrationWorkers: got %d, want 4", cfg.HydrationWorkers)
	}
}

// TestHydrationConfigCacheMaxBytesZero verifies that HydrationCacheMaxBytes=0
// is accepted (it means unlimited).
func TestHydrationConfigCacheMaxBytesZero(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_HYDRATION_CACHE_MAX_BYTES", "0")

	_, err := config.Load("")
	if err != nil {
		t.Errorf("HydrationCacheMaxBytes=0 (unlimited) should be valid: %v", err)
	}
}

// TestHydrationConfigValidation verifies that non-positive values for positive-
// integer fields are rejected, and that a negative CacheMaxBytes is rejected.
func TestHydrationConfigValidation(t *testing.T) {
	t.Run("idle_timeout_s=0", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("JAMSESH_HYDRATION_IDLE_TIMEOUT_S", "0")
		_, err := config.Load("")
		if err == nil {
			t.Error("expected error for JAMSESH_HYDRATION_IDLE_TIMEOUT_S=0, got nil")
		}
	})

	t.Run("idle_timeout_s=-1", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("JAMSESH_HYDRATION_IDLE_TIMEOUT_S", "-1")
		_, err := config.Load("")
		if err == nil {
			t.Error("expected error for JAMSESH_HYDRATION_IDLE_TIMEOUT_S=-1, got nil")
		}
	})

	t.Run("idle_check_period_s=0", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S", "0")
		_, err := config.Load("")
		if err == nil {
			t.Error("expected error for JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S=0, got nil")
		}
	})

	t.Run("workers=0", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("JAMSESH_HYDRATION_WORKERS", "0")
		_, err := config.Load("")
		if err == nil {
			t.Error("expected error for JAMSESH_HYDRATION_WORKERS=0, got nil")
		}
	})

	t.Run("cache_max_bytes=-1", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("JAMSESH_HYDRATION_CACHE_MAX_BYTES", "-1")
		_, err := config.Load("")
		if err == nil {
			t.Error("expected error for JAMSESH_HYDRATION_CACHE_MAX_BYTES=-1, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// Playground config tests
// ---------------------------------------------------------------------------

// TestPlaygroundDefaults verifies that all playground fields have the correct
// default values when no env vars or YAML keys are set.
func TestPlaygroundDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PlaygroundEnabled != false {
		t.Errorf("PlaygroundEnabled default: got %v, want false", cfg.PlaygroundEnabled)
	}
	if cfg.PlaygroundIdleTimeoutS != 1800 {
		t.Errorf("PlaygroundIdleTimeoutS default: got %d, want 1800", cfg.PlaygroundIdleTimeoutS)
	}
	if cfg.PlaygroundHardCapS != 86400 {
		t.Errorf("PlaygroundHardCapS default: got %d, want 86400", cfg.PlaygroundHardCapS)
	}
	if cfg.PlaygroundCreatePerIPHour != 3 {
		t.Errorf("PlaygroundCreatePerIPHour default: got %d, want 3", cfg.PlaygroundCreatePerIPHour)
	}
	if cfg.PlaygroundMaxParticipants != 5 {
		t.Errorf("PlaygroundMaxParticipants default: got %d, want 5", cfg.PlaygroundMaxParticipants)
	}
	if cfg.PlaygroundMaxContentBytes != 52428800 {
		t.Errorf("PlaygroundMaxContentBytes default: got %d, want 52428800", cfg.PlaygroundMaxContentBytes)
	}
	if cfg.PlaygroundDestructionSweepIntervalS != 60 {
		t.Errorf("PlaygroundDestructionSweepIntervalS default: got %d, want 60", cfg.PlaygroundDestructionSweepIntervalS)
	}
}

// TestPlaygroundEnabledEnvOverride verifies JAMSESH_PLAYGROUND_ENABLED
// accepts "true", "1", and "yes" for enabled.
func TestPlaygroundEnabledEnvOverride(t *testing.T) {
	for _, val := range []string{"true", "1", "yes"} {
		val := val
		t.Run(val, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("JAMSESH_PLAYGROUND_ENABLED", val)
			cfg, err := config.Load("")
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !cfg.PlaygroundEnabled {
				t.Errorf("PlaygroundEnabled: got false, want true (from %q)", val)
			}
		})
	}
	// "false" and other strings should leave PlaygroundEnabled=false.
	for _, val := range []string{"false", "0", "no", "off"} {
		val := val
		t.Run(val, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("JAMSESH_PLAYGROUND_ENABLED", val)
			cfg, err := config.Load("")
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.PlaygroundEnabled {
				t.Errorf("PlaygroundEnabled: got true, want false (from %q)", val)
			}
		})
	}
}

// TestPlaygroundEnvOverrides verifies all numeric playground env vars.
func TestPlaygroundEnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S", "600")
	t.Setenv("JAMSESH_PLAYGROUND_HARD_CAP_S", "3600")
	t.Setenv("JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR", "10")
	t.Setenv("JAMSESH_PLAYGROUND_MAX_PARTICIPANTS", "20")
	t.Setenv("JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES", "104857600")
	t.Setenv("JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S", "30")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PlaygroundIdleTimeoutS != 600 {
		t.Errorf("PlaygroundIdleTimeoutS: got %d, want 600", cfg.PlaygroundIdleTimeoutS)
	}
	if cfg.PlaygroundHardCapS != 3600 {
		t.Errorf("PlaygroundHardCapS: got %d, want 3600", cfg.PlaygroundHardCapS)
	}
	if cfg.PlaygroundCreatePerIPHour != 10 {
		t.Errorf("PlaygroundCreatePerIPHour: got %d, want 10", cfg.PlaygroundCreatePerIPHour)
	}
	if cfg.PlaygroundMaxParticipants != 20 {
		t.Errorf("PlaygroundMaxParticipants: got %d, want 20", cfg.PlaygroundMaxParticipants)
	}
	if cfg.PlaygroundMaxContentBytes != 104857600 {
		t.Errorf("PlaygroundMaxContentBytes: got %d, want 104857600", cfg.PlaygroundMaxContentBytes)
	}
	if cfg.PlaygroundDestructionSweepIntervalS != 30 {
		t.Errorf("PlaygroundDestructionSweepIntervalS: got %d, want 30", cfg.PlaygroundDestructionSweepIntervalS)
	}
}

// TestPlaygroundYAML verifies playground fields can be set via YAML.
func TestPlaygroundYAML(t *testing.T) {
	clearEnv(t)
	yamlContent := `
playground_enabled: true
playground_idle_timeout_s: 900
playground_hard_cap_s: 7200
playground_create_per_ip_hour: 5
playground_max_participants: 8
playground_max_content_bytes: 10485760
playground_destruction_sweep_interval_s: 120
`
	path := writeTempConfig(t, yamlContent)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.PlaygroundEnabled {
		t.Error("PlaygroundEnabled: got false, want true")
	}
	if cfg.PlaygroundIdleTimeoutS != 900 {
		t.Errorf("PlaygroundIdleTimeoutS: got %d, want 900", cfg.PlaygroundIdleTimeoutS)
	}
	if cfg.PlaygroundHardCapS != 7200 {
		t.Errorf("PlaygroundHardCapS: got %d, want 7200", cfg.PlaygroundHardCapS)
	}
	if cfg.PlaygroundCreatePerIPHour != 5 {
		t.Errorf("PlaygroundCreatePerIPHour: got %d, want 5", cfg.PlaygroundCreatePerIPHour)
	}
	if cfg.PlaygroundMaxParticipants != 8 {
		t.Errorf("PlaygroundMaxParticipants: got %d, want 8", cfg.PlaygroundMaxParticipants)
	}
	if cfg.PlaygroundMaxContentBytes != 10485760 {
		t.Errorf("PlaygroundMaxContentBytes: got %d, want 10485760", cfg.PlaygroundMaxContentBytes)
	}
	if cfg.PlaygroundDestructionSweepIntervalS != 120 {
		t.Errorf("PlaygroundDestructionSweepIntervalS: got %d, want 120", cfg.PlaygroundDestructionSweepIntervalS)
	}
}

// TestLandingVariantDefault verifies that Landing.Variant defaults to "auto"
// when JAMSESH_LANDING_VARIANT is unset and no YAML key is provided.
func TestLandingVariantDefault(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Landing.Variant != "auto" {
		t.Errorf("Landing.Variant default: got %q, want %q", cfg.Landing.Variant, "auto")
	}
}

// TestLandingVariantEnvOverride verifies that JAMSESH_LANDING_VARIANT=project
// is parsed correctly and overrides the default.
func TestLandingVariantEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_LANDING_VARIANT", "project")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Landing.Variant != "project" {
		t.Errorf("Landing.Variant: got %q, want %q", cfg.Landing.Variant, "project")
	}
}

// TestLandingVariantYAML verifies that landing.variant can be set via YAML.
func TestLandingVariantYAML(t *testing.T) {
	clearEnv(t)
	yamlContent := `
landing:
  variant: login
`
	path := writeTempConfig(t, yamlContent)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Landing.Variant != "login" {
		t.Errorf("Landing.Variant: got %q, want %q", cfg.Landing.Variant, "login")
	}
}

// TestLandingVariantEnvTakesPrecedenceOverYAML verifies env overrides YAML.
func TestLandingVariantEnvTakesPrecedenceOverYAML(t *testing.T) {
	clearEnv(t)
	yamlContent := `
landing:
  variant: login
`
	path := writeTempConfig(t, yamlContent)
	t.Setenv("JAMSESH_LANDING_VARIANT", "project")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Landing.Variant != "project" {
		t.Errorf("Landing.Variant: got %q, want %q (env should win)", cfg.Landing.Variant, "project")
	}
}

// TestValidation_LandingVariantInvalid verifies that an invalid value fails startup.
func TestValidation_LandingVariantInvalid(t *testing.T) {
	clearEnv(t)
	t.Setenv("JAMSESH_LANDING_VARIANT", "invalid")
	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected error for invalid JAMSESH_LANDING_VARIANT, got nil")
	}
}

// TestValidation_LandingVariantValidValues verifies that all valid values are accepted.
func TestValidation_LandingVariantValidValues(t *testing.T) {
	for _, v := range []string{"auto", "project", "login"} {
		t.Run(v, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("JAMSESH_LANDING_VARIANT", v)
			_, err := config.Load("")
			if err != nil {
				t.Fatalf("JAMSESH_LANDING_VARIANT=%q should be valid, got error: %v", v, err)
			}
		})
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
