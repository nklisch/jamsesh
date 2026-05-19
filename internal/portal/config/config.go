// Package config loads and validates the portal server configuration.
// Config is sourced from an optional YAML file with env-var overrides.
//
// YAML keys:  bind, db_driver, db_dsn, portal_url, tls.mode, tls.cert_path,
//
//	tls.key_path, log.format, log.level, storage,
//	email.provider, email.from, email.smtp.*, email.sendgrid.*,
//	email.postmark.*, email.resend.*,
//	oauth.github.client_id, oauth.github.client_secret,
//	oauth.github.base_url,
//	auth_rate_limit_enabled,
//	git.max_pack_bytes, git.receive_pack_max_concurrent,
//	db.max_open_conns, db.max_idle_conns, db.conn_max_lifetime,
//	shutdown_grace_s,
//	deploy_mode, lease_heartbeat_interval_s,
//	lease_retention_days, lease_retention_interval_hours,
//	object_storage_url, object_storage_region,
//	object_storage_endpoint_url, object_storage_path_style,
//	object_storage_sync_queue_size,
//	hydration_idle_timeout_s, hydration_cache_max_bytes,
//	hydration_idle_check_period_s, hydration_workers,
//	metrics_token,
//	api_body_limit_bytes
//
// Env vars:   JAMSESH_AUTH_RATE_LIMIT_ENABLED,
//
//	JAMSESH_BIND, JAMSESH_DB_DRIVER, JAMSESH_DB_DSN,
//
//	JAMSESH_API_BODY_LIMIT_BYTES,
//	JAMSESH_METRICS_TOKEN,
//	JAMSESH_PORTAL_URL,
//	JAMSESH_TLS_MODE, JAMSESH_TLS_CERT, JAMSESH_TLS_KEY,
//	JAMSESH_LOG_FORMAT, JAMSESH_LOG_LEVEL, JAMSESH_STORAGE,
//	JAMSESH_EMAIL_PROVIDER, JAMSESH_EMAIL_FROM,
//	JAMSESH_EMAIL_SMTP_HOST, JAMSESH_EMAIL_SMTP_PORT,
//	JAMSESH_EMAIL_SMTP_USER, JAMSESH_EMAIL_SMTP_PASS,
//	JAMSESH_EMAIL_SMTP_TLS,
//	JAMSESH_EMAIL_SENDGRID_API_KEY,
//	JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN,
//	JAMSESH_EMAIL_POSTMARK_MESSAGE_STREAM,
//	JAMSESH_EMAIL_RESEND_API_KEY,
//	JAMSESH_OAUTH_GITHUB_CLIENT_ID,
//	JAMSESH_OAUTH_GITHUB_CLIENT_SECRET,
//	JAMSESH_OAUTH_GITHUB_BASE_URL,
//	JAMSESH_GIT_MAX_PACK_BYTES,
//	JAMSESH_RECEIVE_PACK_MAX_CONCURRENT,
//	JAMSESH_DB_MAX_OPEN_CONNS,
//	JAMSESH_DB_MAX_IDLE_CONNS,
//	JAMSESH_DB_CONN_MAX_LIFETIME,
//	JAMSESH_SHUTDOWN_GRACE_S,
//	JAMSESH_DEPLOY_MODE,
//	JAMSESH_LEASE_HEARTBEAT_INTERVAL_S,
//	JAMSESH_LEASE_RETENTION_DAYS,
//	JAMSESH_LEASE_RETENTION_INTERVAL_HOURS,
//	JAMSESH_OBJECT_STORAGE_URL,
//	JAMSESH_OBJECT_STORAGE_REGION,
//	JAMSESH_OBJECT_STORAGE_ENDPOINT_URL,
//	JAMSESH_OBJECT_STORAGE_PATH_STYLE,
//	JAMSESH_OBJECT_STORAGE_SYNC_QUEUE_SIZE,
//	JAMSESH_HYDRATION_IDLE_TIMEOUT_S,
//	JAMSESH_HYDRATION_CACHE_MAX_BYTES,
//	JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S,
//	JAMSESH_HYDRATION_WORKERS
//
// Secret env vars with _FILE variants (file contents take precedence):
//
//	JAMSESH_DB_DSN_FILE,
//	JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE,
//	JAMSESH_EMAIL_SMTP_PASS_FILE,
//	JAMSESH_EMAIL_SENDGRID_API_KEY_FILE,
//	JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN_FILE,
//	JAMSESH_EMAIL_RESEND_API_KEY_FILE
//
// log.level is an integer matching slog.Level values:
//
//	-4=DEBUG, 0=INFO, 4=WARN, 8=ERROR
//
// Use JAMSESH_LOG_LEVEL=-4 for debug, 0 for info (default), etc.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the full portal server configuration.
type Config struct {
	Bind      string      `yaml:"bind"`       // listen address, e.g. ":8443"
	DBDriver  string      `yaml:"db_driver"`  // "sqlite" | "postgres"
	DBDSN     string      `yaml:"db_dsn"`     // DSN appropriate for DBDriver
	PortalURL string      `yaml:"portal_url"` // public base URL, e.g. "https://example.com"
	TLS       TLSConfig   `yaml:"tls"`
	Log       LogConfig   `yaml:"log"`
	Storage   string      `yaml:"storage"` // path for bare git repos
	Email     EmailConfig `yaml:"email"`
	OAuth     OAuthConfig `yaml:"oauth"`
	Git       GitConfig   `yaml:"git"`
	DB        DBConfig    `yaml:"db"`
	// ShutdownGraceSeconds is the total wall-clock budget for graceful shutdown,
	// shared across HTTP draining, auto-merger stop, and WS gateway stop.
	// Default: 30 (matches k8s terminationGracePeriodSeconds). Must be positive.
	// Env: JAMSESH_SHUTDOWN_GRACE_S
	ShutdownGraceSeconds int `yaml:"shutdown_grace_s"`

	// DeployMode selects single-instance vs. clustered operation.
	// "single" (default) uses NoopManager — no PG lease queries are issued.
	// "clustered" uses PostgresManager with advisory locks and fencing tokens;
	// requires db_driver=postgres.
	// Env: JAMSESH_DEPLOY_MODE
	DeployMode string `yaml:"deploy_mode"`

	// LeaseHeartbeatIntervalS is how often (in seconds) the heartbeat goroutine
	// pings the dedicated PG connection. Default: 10.
	// Env: JAMSESH_LEASE_HEARTBEAT_INTERVAL_S
	LeaseHeartbeatIntervalS int `yaml:"lease_heartbeat_interval_s"`

	// LeaseRetentionDays controls how long released lease rows are kept before
	// the retention goroutine deletes them. Default: 30.
	// Env: JAMSESH_LEASE_RETENTION_DAYS
	LeaseRetentionDays int `yaml:"lease_retention_days"`

	// LeaseRetentionIntervalHours is how often the retention goroutine runs.
	// Default: 1.
	// Env: JAMSESH_LEASE_RETENTION_INTERVAL_HOURS
	LeaseRetentionIntervalHours int `yaml:"lease_retention_interval_hours"`

	// ObjectStorageURL is the object-storage location used as the system of
	// record in clustered mode. Format: <scheme>://bucket[/optional-prefix].
	//
	// Supported schemes:
	//   s3://bucket/prefix              — AWS S3 (IRSA or env creds)
	//   s3-compatible://bucket/prefix   — S3-compatible endpoint (e.g. R2, MinIO)
	//   gs://bucket/prefix              — Google Cloud Storage
	//   azblob://account/container/prefix — Azure Blob Storage
	//
	// Required when DeployMode == "clustered". The portal refuses to start
	// in clustered mode without an object-storage URL because bare-repo
	// durability depends on it.
	// Env: JAMSESH_OBJECT_STORAGE_URL
	ObjectStorageURL string `yaml:"object_storage_url"`

	// ObjectStorageRegion is the provider region (e.g. "us-east-1" for AWS,
	// "us-central1" for GCS). Required for AWS S3; optional for S3-compatible
	// services and ignored by GCS / Azure.
	// Env: JAMSESH_OBJECT_STORAGE_REGION
	ObjectStorageRegion string `yaml:"object_storage_region"`

	// ObjectStorageEndpointURL overrides the default provider endpoint. Set
	// this for Cloudflare R2, Backblaze B2, MinIO, or any other S3-compatible
	// service that requires a custom endpoint. Leave empty for native AWS S3,
	// GCS, and Azure Blob.
	// Example: "https://<account>.r2.cloudflarestorage.com"
	// Env: JAMSESH_OBJECT_STORAGE_ENDPOINT_URL
	ObjectStorageEndpointURL string `yaml:"object_storage_endpoint_url"`

	// ObjectStoragePathStyle forces path-style bucket addressing
	// (http://host/bucket/key instead of http://bucket.host/key). Required
	// for MinIO and self-hosted Ceph; set false for AWS S3 and Cloudflare R2.
	// Env: JAMSESH_OBJECT_STORAGE_PATH_STYLE (accepted values: "true", "false")
	ObjectStoragePathStyle bool `yaml:"object_storage_path_style"`

	// ObjectStorageSyncQueueSize is the maximum number of concurrent in-flight
	// SyncPush calls allowed per session. When the limit is reached, additional
	// pushes receive 503 Retry-After until uploads catch up.
	// Default: 256. Must be positive.
	// Env: JAMSESH_OBJECT_STORAGE_SYNC_QUEUE_SIZE
	ObjectStorageSyncQueueSize int `yaml:"object_storage_sync_queue_size"`

	// HydrationIdleTimeoutS is how long (in seconds) a session can be inactive
	// before the LifecycleManager evicts its local cache and releases its lease.
	// Default: 300 (5 minutes). Must be positive.
	// Env: JAMSESH_HYDRATION_IDLE_TIMEOUT_S
	HydrationIdleTimeoutS int `yaml:"hydration_idle_timeout_s"`

	// HydrationCacheMaxBytes is the maximum cumulative bytes of all active
	// per-session bare repos on local disk. When the total exceeds this value,
	// the least-recently-active session is evicted (LRU). Zero means unlimited.
	// Default: 0 (unlimited).
	// Env: JAMSESH_HYDRATION_CACHE_MAX_BYTES
	HydrationCacheMaxBytes int64 `yaml:"hydration_cache_max_bytes"`

	// HydrationIdleCheckPeriodS is how often (in seconds) the idle-eviction and
	// LRU loops run. Default: 30. Must be positive.
	// Env: JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S
	HydrationIdleCheckPeriodS int `yaml:"hydration_idle_check_period_s"`

	// HydrationWorkers is the number of parallel download workers used by the
	// Hydrator when re-seeding a session's bare repo from object storage.
	// Default: 8. Must be positive.
	// Env: JAMSESH_HYDRATION_WORKERS
	HydrationWorkers int `yaml:"hydration_workers"`

	// AuthRateLimitEnabled controls whether per-IP rate limiting is applied to
	// the unauthenticated /auth/* endpoints (magic-link request/exchange,
	// oauth/start, oauth/callback, refresh). Default: true.
	// Set JAMSESH_AUTH_RATE_LIMIT_ENABLED=false to disable for single-user
	// self-host scenarios where email-bombing is not a concern.
	// Env: JAMSESH_AUTH_RATE_LIMIT_ENABLED
	AuthRateLimitEnabled bool `yaml:"auth_rate_limit_enabled"`

	// MetricsToken is the static bearer token required to access GET /metrics.
	// When unset (the default), the /metrics route is not registered — operators
	// must explicitly opt in by setting this value. When set, requests to
	// /metrics must supply "Authorization: Bearer <token>"; missing or wrong
	// tokens receive 401.
	// Env: JAMSESH_METRICS_TOKEN
	MetricsToken string `yaml:"metrics_token"`

	// APIBodyLimitBytes is the maximum number of bytes the server will read
	// from any request body on /api/* routes before returning 413. Zero means
	// use the built-in default of 1 MiB (1 << 20 = 1048576).
	// Env: JAMSESH_API_BODY_LIMIT_BYTES
	APIBodyLimitBytes int64 `yaml:"api_body_limit_bytes"`
}

// DBConfig holds database connection pool settings.
// These apply to Postgres; for SQLite the values are accepted but have no
// concurrency benefit since SQLite is effectively single-writer.
type DBConfig struct {
	// MaxOpenConns is the maximum number of open connections in the pool.
	// Default: 25. For Postgres this maps to pgxpool.Config.MaxConns.
	MaxOpenConns int `yaml:"max_open_conns"`
	// MaxIdleConns is the minimum number of idle connections the pool
	// maintains. Default: 5. For Postgres this maps to pgxpool.Config.MinConns.
	MaxIdleConns int `yaml:"max_idle_conns"`
	// ConnMaxLifetime is the maximum lifetime of a pooled connection before
	// it is closed and replaced. Default: 30m.
	// Accepts Go duration strings: "30m", "1h", etc.
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

// GitConfig holds git-push policy settings.
type GitConfig struct {
	// MaxPackBytes is the maximum size (in bytes) of a pushed pack.
	// Pushes exceeding this limit are rejected with push.size_limit.
	// Default: 52428800 (50 MiB). Set to 0 to disable the check.
	MaxPackBytes int64 `yaml:"max_pack_bytes"`
	// ReceivePackMaxConcurrent is the maximum number of concurrent
	// git-receive-pack handlers allowed per portal instance. Requests that
	// arrive when the semaphore is full are rejected with 503 Retry-After
	// so the git client can retry. Default: 4.
	// Env: JAMSESH_RECEIVE_PACK_MAX_CONCURRENT
	ReceivePackMaxConcurrent int `yaml:"receive_pack_max_concurrent"`
}

// OAuthConfig holds OAuth provider credentials. Only providers with non-empty
// ClientID and ClientSecret are considered configured. The start endpoint
// returns 503 for providers without credentials.
type OAuthConfig struct {
	GitHub GitHubOAuthConfig `yaml:"github"`
}

// GitHubOAuthConfig holds GitHub OAuth application credentials.
type GitHubOAuthConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	// BaseURL overrides the GitHub OAuth and API base URL for testing.
	// Leave empty in production.
	BaseURL string `yaml:"base_url"`
}

// EmailConfig selects the email delivery provider and holds all provider
// credentials. Only the chosen provider's sub-struct needs valid fields;
// the others are ignored. Validation runs in senders.New() at startup.
type EmailConfig struct {
	// Provider selects the delivery backend: smtp | sendgrid | postmark | resend
	Provider string `yaml:"provider"`
	// From is the envelope sender address, e.g. "jamsesh <noreply@example.com>"
	From string `yaml:"from"`

	SMTP     SMTPConfig     `yaml:"smtp"`
	SendGrid SendGridConfig `yaml:"sendgrid"`
	Postmark PostmarkConfig `yaml:"postmark"`
	Resend   ResendConfig   `yaml:"resend"`
}

// SMTPConfig holds SMTP connection settings.
type SMTPConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	User    string `yaml:"user"`
	Pass    string `yaml:"pass"`
	TLSMode string `yaml:"tls"` // "mandatory" | "opportunistic" | "none"
}

// SendGridConfig holds SendGrid API credentials.
type SendGridConfig struct {
	APIKey string `yaml:"api_key"`
}

// PostmarkConfig holds Postmark API credentials.
type PostmarkConfig struct {
	ServerToken   string `yaml:"server_token"`
	MessageStream string `yaml:"message_stream"` // defaults to "outbound"
}

// ResendConfig holds Resend API credentials.
type ResendConfig struct {
	APIKey string `yaml:"api_key"`
}

// TLSConfig controls how the portal presents to clients.
type TLSConfig struct {
	Mode     string `yaml:"mode"`      // "native" | "behind_proxy"
	CertPath string `yaml:"cert_path"` // required when mode == "native"
	KeyPath  string `yaml:"key_path"`  // required when mode == "native"
}

// LogConfig controls structured logging output.
// Level is an integer: -4=DEBUG, 0=INFO, 4=WARN, 8=ERROR (slog.Level).
// YAML accepts both integer and string representations:
//
//	log.level: -4       # DEBUG
//	log.level: "debug"  # also DEBUG (case-insensitive)
type LogConfig struct {
	Format string     `yaml:"format"` // "json" | "text"
	Level  slog.Level `yaml:"level"`  // integer or level name; see package doc
}

// UnmarshalYAML handles both integer and string level values in YAML config.
// String names follow slog's own convention (e.g. "DEBUG", "INFO", "WARN",
// "ERROR") and are accepted case-insensitively. Integer values are used
// directly as slog.Level, allowing fine-grained levels like -4, 0, 4, 8.
func (lc *LogConfig) UnmarshalYAML(value *yaml.Node) error {
	// Use an intermediate struct to capture the raw level node.
	type plain struct {
		Format string    `yaml:"format"`
		Level  yaml.Node `yaml:"level"`
	}
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	lc.Format = p.Format
	if p.Level.Value == "" {
		// level key absent; leave Level at zero value (INFO = 0).
		return nil
	}
	// Try integer first.
	if n, err := strconv.Atoi(p.Level.Value); err == nil {
		lc.Level = slog.Level(n)
		return nil
	}
	// Fall back to slog's own text parsing (handles DEBUG/INFO/WARN/ERROR).
	if err := lc.Level.UnmarshalText([]byte(p.Level.Value)); err != nil {
		return fmt.Errorf("config: log.level %q: %w", p.Level.Value, err)
	}
	return nil
}

// Load reads configuration from an optional YAML file at path, then
// overlays environment variables. Returns validated defaults when path
// is empty and no env vars are set. Returns an error if the file cannot
// be read/parsed or if the resulting config fails validation.
func Load(path string) (Config, error) {
	cfg := defaults()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("config: read %s: %w", path, err)
		}
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("config: parse %s: %w", path, err)
		}
	}
	if err := applyEnv(&cfg); err != nil {
		return cfg, err
	}
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// defaults returns the baseline configuration that matches docs/SELF_HOST.md
// Configuration table.
func defaults() Config {
	return Config{
		Bind:      ":8443",
		DBDriver:  "sqlite",
		DBDSN:     "./jamsesh.db",
		PortalURL: "http://localhost:8443",
		TLS:       TLSConfig{Mode: "behind_proxy"},
		Log:       LogConfig{Format: "json", Level: slog.LevelInfo},
		Storage:   "./storage",
		Email: EmailConfig{
			// Provider defaults to empty so that deployments without an email
			// provider configured (OAuth-only, no-auth) start cleanly without
			// requiring JAMSESH_EMAIL_FROM. When the operator sets
			// JAMSESH_EMAIL_PROVIDER, the full validation (including
			// JAMSESH_EMAIL_FROM) runs at startup via senders.New.
			SMTP: SMTPConfig{
				Host:    "localhost",
				Port:    587,
				TLSMode: "mandatory",
			},
		},
		Git: GitConfig{
			MaxPackBytes:             52428800, // 50 MiB
			ReceivePackMaxConcurrent: 4,
		},
		DB: DBConfig{
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 30 * time.Minute,
		},
		ShutdownGraceSeconds:        30,
		DeployMode:                  "single",
		LeaseHeartbeatIntervalS:     10,
		LeaseRetentionDays:          30,
		LeaseRetentionIntervalHours: 1,
		ObjectStorageSyncQueueSize:  256,
		HydrationIdleTimeoutS:       300,
		HydrationCacheMaxBytes:      0,
		HydrationIdleCheckPeriodS:   30,
		HydrationWorkers:            8,
		AuthRateLimitEnabled:        true,
	}
}

// validate checks invariants that cannot be expressed as YAML types.
func (c Config) validate() error {
	switch c.TLS.Mode {
	case "native":
		if c.TLS.CertPath == "" || c.TLS.KeyPath == "" {
			return fmt.Errorf("config: tls.mode=native requires tls.cert_path and tls.key_path")
		}
	case "behind_proxy":
		// valid; no TLS material needed
	default:
		return fmt.Errorf("config: tls.mode must be \"native\" or \"behind_proxy\", got %q", c.TLS.Mode)
	}
	switch c.DBDriver {
	case "sqlite", "postgres":
		// valid
	default:
		return fmt.Errorf("config: db_driver must be \"sqlite\" or \"postgres\", got %q", c.DBDriver)
	}
	if c.ShutdownGraceSeconds <= 0 {
		return fmt.Errorf("config: shutdown_grace_s must be a positive integer, got %d", c.ShutdownGraceSeconds)
	}
	switch c.DeployMode {
	case "single", "clustered":
		// valid
	default:
		return fmt.Errorf("config: deploy_mode must be \"single\" or \"clustered\", got %q", c.DeployMode)
	}
	if c.DeployMode == "clustered" && c.DBDriver == "sqlite" {
		return fmt.Errorf("config: deploy_mode=clustered requires db_driver=postgres; SQLite does not support distributed leases")
	}
	if c.DeployMode == "clustered" && c.ObjectStorageURL == "" {
		return fmt.Errorf("config: deploy_mode=clustered requires object_storage_url (JAMSESH_OBJECT_STORAGE_URL); " +
			"bare-repo durability in clustered mode depends on object storage — " +
			"set JAMSESH_OBJECT_STORAGE_URL to an s3://, s3-compatible://, gs://, or azblob:// URL")
	}
	if c.ObjectStorageSyncQueueSize <= 0 {
		return fmt.Errorf("config: object_storage_sync_queue_size must be a positive integer, got %d", c.ObjectStorageSyncQueueSize)
	}
	if c.LeaseHeartbeatIntervalS <= 0 {
		return fmt.Errorf("config: lease_heartbeat_interval_s must be a positive integer, got %d", c.LeaseHeartbeatIntervalS)
	}
	if c.LeaseRetentionDays <= 0 {
		return fmt.Errorf("config: lease_retention_days must be a positive integer, got %d", c.LeaseRetentionDays)
	}
	if c.LeaseRetentionIntervalHours <= 0 {
		return fmt.Errorf("config: lease_retention_interval_hours must be a positive integer, got %d", c.LeaseRetentionIntervalHours)
	}
	if c.HydrationIdleTimeoutS <= 0 {
		return fmt.Errorf("config: hydration_idle_timeout_s must be a positive integer, got %d", c.HydrationIdleTimeoutS)
	}
	if c.HydrationIdleCheckPeriodS <= 0 {
		return fmt.Errorf("config: hydration_idle_check_period_s must be a positive integer, got %d", c.HydrationIdleCheckPeriodS)
	}
	if c.HydrationWorkers <= 0 {
		return fmt.Errorf("config: hydration_workers must be a positive integer, got %d", c.HydrationWorkers)
	}
	if c.HydrationCacheMaxBytes < 0 {
		return fmt.Errorf("config: hydration_cache_max_bytes must be zero or positive, got %d", c.HydrationCacheMaxBytes)
	}
	if c.Git.ReceivePackMaxConcurrent <= 0 {
		return fmt.Errorf("config: git.receive_pack_max_concurrent must be a positive integer, got %d", c.Git.ReceivePackMaxConcurrent)
	}
	return nil
}

// applyEnv overlays environment variables onto cfg. Only non-empty env
// values take effect; missing vars leave the existing value unchanged.
// Returns an error if a _FILE secret variable is set but unreadable.
func applyEnv(c *Config) error {
	if v := os.Getenv("JAMSESH_BIND"); v != "" {
		c.Bind = v
	}
	if v := os.Getenv("JAMSESH_DB_DRIVER"); v != "" {
		c.DBDriver = v
	}
	v, err := readEnvOrFile("JAMSESH_DB_DSN")
	if err != nil {
		return err
	}
	if v != "" {
		c.DBDSN = v
	}
	if v := os.Getenv("JAMSESH_TLS_MODE"); v != "" {
		c.TLS.Mode = v
	}
	if v := os.Getenv("JAMSESH_TLS_CERT"); v != "" {
		c.TLS.CertPath = v
	}
	if v := os.Getenv("JAMSESH_TLS_KEY"); v != "" {
		c.TLS.KeyPath = v
	}
	if v := os.Getenv("JAMSESH_LOG_FORMAT"); v != "" {
		c.Log.Format = v
	}
	if v := os.Getenv("JAMSESH_LOG_LEVEL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Log.Level = slog.Level(n)
		}
	}
	if v := os.Getenv("JAMSESH_STORAGE"); v != "" {
		c.Storage = v
	}
	if v := os.Getenv("JAMSESH_PORTAL_URL"); v != "" {
		c.PortalURL = v
	}
	if err := applyEmailEnv(&c.Email); err != nil {
		return err
	}
	if err := applyOAuthEnv(&c.OAuth); err != nil {
		return err
	}
	applyGitEnv(&c.Git)
	applyDBEnv(&c.DB)
	if v := os.Getenv("JAMSESH_SHUTDOWN_GRACE_S"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.ShutdownGraceSeconds = n
		}
	}
	applyLeaseEnv(c)
	applyObjectStorageEnv(c)
	applyHydrationEnv(c)
	applyAuthRateLimitEnv(c)
	applyMetricsEnv(c)
	applyAPIBodyLimitEnv(c)
	return nil
}

// applyLeaseEnv overlays lease-related environment variables.
func applyLeaseEnv(c *Config) {
	if v := os.Getenv("JAMSESH_DEPLOY_MODE"); v != "" {
		c.DeployMode = v
	}
	if v := os.Getenv("JAMSESH_LEASE_HEARTBEAT_INTERVAL_S"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.LeaseHeartbeatIntervalS = n
		}
	}
	if v := os.Getenv("JAMSESH_LEASE_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.LeaseRetentionDays = n
		}
	}
	if v := os.Getenv("JAMSESH_LEASE_RETENTION_INTERVAL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.LeaseRetentionIntervalHours = n
		}
	}
}

// applyObjectStorageEnv overlays object-storage environment variables.
func applyObjectStorageEnv(c *Config) {
	if v := os.Getenv("JAMSESH_OBJECT_STORAGE_URL"); v != "" {
		c.ObjectStorageURL = v
	}
	if v := os.Getenv("JAMSESH_OBJECT_STORAGE_REGION"); v != "" {
		c.ObjectStorageRegion = v
	}
	if v := os.Getenv("JAMSESH_OBJECT_STORAGE_ENDPOINT_URL"); v != "" {
		c.ObjectStorageEndpointURL = v
	}
	if v := os.Getenv("JAMSESH_OBJECT_STORAGE_PATH_STYLE"); v != "" {
		c.ObjectStoragePathStyle = v == "true"
	}
	if v := os.Getenv("JAMSESH_OBJECT_STORAGE_SYNC_QUEUE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.ObjectStorageSyncQueueSize = n
		}
	}
}

// applyHydrationEnv overlays hydration-lifecycle environment variables.
func applyHydrationEnv(c *Config) {
	if v := os.Getenv("JAMSESH_HYDRATION_IDLE_TIMEOUT_S"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.HydrationIdleTimeoutS = n
		}
	}
	if v := os.Getenv("JAMSESH_HYDRATION_CACHE_MAX_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.HydrationCacheMaxBytes = n
		}
	}
	if v := os.Getenv("JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.HydrationIdleCheckPeriodS = n
		}
	}
	if v := os.Getenv("JAMSESH_HYDRATION_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.HydrationWorkers = n
		}
	}
}

// applyGitEnv overlays git-policy environment variables.
func applyGitEnv(g *GitConfig) {
	if v := os.Getenv("JAMSESH_GIT_MAX_PACK_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			g.MaxPackBytes = n
		}
	}
	if v := os.Getenv("JAMSESH_RECEIVE_PACK_MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			g.ReceivePackMaxConcurrent = n
		}
	}
}

// applyDBEnv overlays database connection pool environment variables.
func applyDBEnv(d *DBConfig) {
	if v := os.Getenv("JAMSESH_DB_MAX_OPEN_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			d.MaxOpenConns = n
		}
	}
	if v := os.Getenv("JAMSESH_DB_MAX_IDLE_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			d.MaxIdleConns = n
		}
	}
	if v := os.Getenv("JAMSESH_DB_CONN_MAX_LIFETIME"); v != "" {
		if dur, err := time.ParseDuration(v); err == nil {
			d.ConnMaxLifetime = dur
		}
	}
}

// applyOAuthEnv overlays OAuth-related environment variables.
// Returns an error if a _FILE secret variable is set but unreadable.
func applyOAuthEnv(o *OAuthConfig) error {
	if v := os.Getenv("JAMSESH_OAUTH_GITHUB_CLIENT_ID"); v != "" {
		o.GitHub.ClientID = v
	}
	v, err := readEnvOrFile("JAMSESH_OAUTH_GITHUB_CLIENT_SECRET")
	if err != nil {
		return err
	}
	if v != "" {
		o.GitHub.ClientSecret = v
	}
	if v := os.Getenv("JAMSESH_OAUTH_GITHUB_BASE_URL"); v != "" {
		o.GitHub.BaseURL = v
	}
	return nil
}

// applyAuthRateLimitEnv overlays auth rate-limit environment variables.
// JAMSESH_AUTH_RATE_LIMIT_ENABLED accepts "true" (default) or "false".
// Any value other than "false" is treated as enabled.
func applyAuthRateLimitEnv(c *Config) {
	if v := os.Getenv("JAMSESH_AUTH_RATE_LIMIT_ENABLED"); v != "" {
		c.AuthRateLimitEnabled = v != "false"
	}
}

// applyMetricsEnv overlays metrics-auth environment variables.
// JAMSESH_METRICS_TOKEN, when set, enables the /metrics endpoint and
// requires requests to present a matching bearer token.
func applyMetricsEnv(c *Config) {
	if v := os.Getenv("JAMSESH_METRICS_TOKEN"); v != "" {
		c.MetricsToken = v
	}
}

// applyAPIBodyLimitEnv overlays the REST body-cap environment variable.
// JAMSESH_API_BODY_LIMIT_BYTES, when set to a positive integer, overrides the
// default 1 MiB cap applied to all /api/* routes.
func applyAPIBodyLimitEnv(c *Config) {
	if v := os.Getenv("JAMSESH_API_BODY_LIMIT_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			c.APIBodyLimitBytes = n
		}
	}
}

// applyEmailEnv overlays email-related environment variables.
// Returns an error if a _FILE secret variable is set but unreadable.
func applyEmailEnv(e *EmailConfig) error {
	if v := os.Getenv("JAMSESH_EMAIL_PROVIDER"); v != "" {
		e.Provider = v
	}
	if v := os.Getenv("JAMSESH_EMAIL_FROM"); v != "" {
		e.From = v
	}
	if v := os.Getenv("JAMSESH_EMAIL_SMTP_HOST"); v != "" {
		e.SMTP.Host = v
	}
	if v := os.Getenv("JAMSESH_EMAIL_SMTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			e.SMTP.Port = n
		}
	}
	if v := os.Getenv("JAMSESH_EMAIL_SMTP_USER"); v != "" {
		e.SMTP.User = v
	}
	smtpPass, err := readEnvOrFile("JAMSESH_EMAIL_SMTP_PASS")
	if err != nil {
		return err
	}
	if smtpPass != "" {
		e.SMTP.Pass = smtpPass
	}
	if v := os.Getenv("JAMSESH_EMAIL_SMTP_TLS"); v != "" {
		e.SMTP.TLSMode = v
	}
	sendgridKey, err := readEnvOrFile("JAMSESH_EMAIL_SENDGRID_API_KEY")
	if err != nil {
		return err
	}
	if sendgridKey != "" {
		e.SendGrid.APIKey = sendgridKey
	}
	postmarkToken, err := readEnvOrFile("JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN")
	if err != nil {
		return err
	}
	if postmarkToken != "" {
		e.Postmark.ServerToken = postmarkToken
	}
	if v := os.Getenv("JAMSESH_EMAIL_POSTMARK_MESSAGE_STREAM"); v != "" {
		e.Postmark.MessageStream = v
	}
	resendKey, err := readEnvOrFile("JAMSESH_EMAIL_RESEND_API_KEY")
	if err != nil {
		return err
	}
	if resendKey != "" {
		e.Resend.APIKey = resendKey
	}
	return nil
}
